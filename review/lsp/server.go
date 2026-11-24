package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pantacor/lawgate/review"
)

// Config says where the repository is and how to index it.
type Config struct {
	// Root is the repository root (the directory of lawgate.json). Empty
	// finds it from the client's workspace folders, then the current
	// directory.
	Root string
	// Build indexes a repository. `lawgate review -lsp` passes review.Build
	// with the check's command runner.
	Build func(root string) (*review.Index, error)
}

// ReindexDelay is how long after the last save of a .go file or a LAWS.md
// the index is rebuilt, so a burst of saves rebuilds once.
const ReindexDelay = time.Second

type server struct {
	config  Config
	root    string
	out     io.Writer
	writeMu sync.Mutex

	mu         sync.Mutex
	idx        *review.Index
	ready      chan struct{} // closed once the first index is built
	docs       map[string]string
	published  map[string]bool // URIs whose diagnostics were last sent
	siteLenses bool
	reindexAt  *time.Timer
}

// Serve answers LSP requests from in on out until the client exits or in
// ends.
func Serve(config Config, in io.Reader, out io.Writer) error {
	s := &server{config: config, out: out, ready: make(chan struct{}), docs: make(map[string]string), published: make(map[string]bool), siteLenses: true}
	defer s.stopReindex()
	reader := bufio.NewReader(in)
	for {
		message, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if message.Method == "exit" {
			return nil
		}
		s.handle(message)
	}
}

func (s *server) write(message rpcMessage) {
	message.JSONRPC = "2.0"
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(data))
	s.out.Write(data)
}

func (s *server) reply(id *json.RawMessage, result any, err error) {
	if id == nil {
		return
	}
	if err != nil {
		failure := &rpcError{Code: codeInternal, Message: err.Error()}
		s.write(rpcMessage{ID: id, Error: failure})
		return
	}
	if result == nil {
		result = json.RawMessage("null")
	}
	s.write(rpcMessage{ID: id, Result: result})
}

func (s *server) notify(method string, params any) {
	data, _ := json.Marshal(params)
	s.write(rpcMessage{Method: method, Params: data})
}

func (s *server) logf(format string, args ...any) {
	message := map[string]any{"type": 3, "message": fmt.Sprintf(format, args...)}
	s.notify("window/logMessage", message)
}

// requests are answered off the read loop: they wait for the first index,
// and commands run git and machine commands.
var requests = map[string]func(*server, json.RawMessage) (any, error){
	"textDocument/hover":       (*server).hover,
	"textDocument/definition":  (*server).definition,
	"textDocument/codeLens":    (*server).codeLenses,
	"workspace/executeCommand": (*server).executeCommand,
}

func (s *server) handle(message *rpcMessage) {
	if answer, found := requests[message.Method]; found {
		go func() {
			result, err := answer(s, message.Params)
			s.reply(message.ID, result, err)
		}()
		return
	}
	switch message.Method {
	case "initialize":
		s.reply(message.ID, s.initialize(message.Params), nil)
	case "initialized":
		go s.whenReady(s.publishAll)
	case "shutdown":
		s.reply(message.ID, nil, nil)
	default:
		s.notification(message)
	}
}

func (s *server) notification(message *rpcMessage) {
	switch message.Method {
	case "textDocument/didOpen":
		s.didOpen(message.Params)
	case "textDocument/didChange":
		s.didChange(message.Params)
	case "textDocument/didClose":
		s.didClose(message.Params)
	case "textDocument/didSave":
		s.didSave(message.Params)
	default:
		s.unknown(message)
	}
}

// unknown refuses a request the server does not serve; responses to its
// own requests and unknown notifications are dropped.
func (s *server) unknown(message *rpcMessage) {
	if message.ID == nil || message.Method == "" {
		return
	}
	failure := &rpcError{Code: codeMethodNotFound, Message: "method not supported: " + message.Method}
	s.write(rpcMessage{ID: message.ID, Error: failure})
}

func (s *server) whenReady(run func()) {
	<-s.ready
	run()
}

// index is the current index, once the first one is built.
func (s *server) index() *review.Index {
	<-s.ready
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

type initializeParams struct {
	RootURI          string       `json:"rootUri"`
	WorkspaceFolders []documentID `json:"workspaceFolders"`
	Options          struct {
		SiteLenses *bool `json:"siteLenses"`
	} `json:"initializationOptions"`
}

func (s *server) initialize(params json.RawMessage) any {
	var p initializeParams
	_ = json.Unmarshal(params, &p)
	if p.Options.SiteLenses != nil {
		s.siteLenses = *p.Options.SiteLenses
	}
	s.root = s.findRoot(p)
	go s.firstIndex()
	info := map[string]any{"name": "lawgate"}
	return map[string]any{"capabilities": capabilities(), "serverInfo": info}
}

func capabilities() map[string]any {
	save := map[string]any{"includeText": false}
	documentSync := map[string]any{"openClose": true, "change": 1, "save": save}
	lenses := map[string]any{"resolveProvider": false}
	// The commands are not listed: vscode-languageclient would register each
	// as an editor command and fail on its next restart ("command already
	// exists"). Clients call workspace/executeCommand directly.
	commands := map[string]any{"commands": []string{}}
	return map[string]any{
		"textDocumentSync":       documentSync,
		"hoverProvider":          true,
		"definitionProvider":     true,
		"codeLensProvider":       lenses,
		"executeCommandProvider": commands,
	}
}

// findRoot picks the repository: the configured root, else the first
// workspace folder (or root URI, or current directory) under a lawgate.json.
func (s *server) findRoot(p initializeParams) string {
	if s.config.Root != "" {
		return s.config.Root
	}
	var starts []string
	for _, folder := range p.WorkspaceFolders {
		starts = append(starts, uriPath(folder.URI))
	}
	if p.RootURI != "" {
		starts = append(starts, uriPath(p.RootURI))
	}
	for _, start := range append(starts, ".") {
		if root, err := review.FindRoot(start); err == nil {
			return root
		}
	}
	absolute, _ := filepath.Abs(".")
	return absolute
}

func (s *server) firstIndex() {
	s.setIndex(s.build())
	close(s.ready)
	s.announce()
}

// build indexes the repository; a failure is logged and leaves an empty
// index, so the editor keeps working.
func (s *server) build() *review.Index {
	idx, err := s.config.Build(s.root)
	if err != nil {
		s.logf("lawgate: index failed: %v", err)
		return &review.Index{Root: s.root, Laws: make(map[string]*review.Law)}
	}
	return idx
}

func (s *server) setIndex(idx *review.Index) {
	s.mu.Lock()
	s.idx = idx
	s.mu.Unlock()
}

// announce tells the client a new index is in: diagnostics, a code lens
// refresh, and the lawgate/indexChanged summary.
func (s *server) announce() {
	s.publishAll()
	s.write(rpcMessage{ID: rawID("refresh"), Method: "workspace/codeLens/refresh"})
	s.notify("lawgate/indexChanged", s.index().Summarize())
}

// scheduleReindex rebuilds the index ReindexDelay after the last save.
func (s *server) scheduleReindex() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reindexAt != nil {
		s.reindexAt.Stop()
	}
	s.reindexAt = time.AfterFunc(ReindexDelay, func() { s.reindex() })
}

func (s *server) stopReindex() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reindexAt != nil {
		s.reindexAt.Stop()
	}
}

// reindex rebuilds the index: the machine commands run again, so a changed
// declaration is compiled in.
func (s *server) reindex() review.Summary {
	<-s.ready
	s.setIndex(s.build())
	s.announce()
	return s.index().Summarize()
}

func (s *server) didOpen(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"textDocument"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	s.setDocument(p.TextDocument.URI, p.TextDocument.Text)
	go s.whenReady(s.publishAll)
}

func (s *server) didChange(params json.RawMessage) {
	var p struct {
		TextDocument   documentID `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if json.Unmarshal(params, &p) != nil || len(p.ContentChanges) == 0 {
		return
	}
	s.setDocument(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text)
}

func (s *server) setDocument(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = text
}

func (s *server) didClose(params json.RawMessage) {
	var p documentParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, p.TextDocument.URI)
}

func (s *server) didSave(params json.RawMessage) {
	var p documentParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	path := uriPath(p.TextDocument.URI)
	if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "LAWS.md") {
		s.scheduleReindex()
	}
}

// rel turns a document URI into a path relative to the repository root.
func (s *server) rel(uri string) (string, bool) {
	relative, err := filepath.Rel(s.root, uriPath(uri))
	if err != nil || !filepath.IsLocal(relative) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func (s *server) uri(relative string) string {
	return pathURI(filepath.Join(s.root, filepath.FromSlash(relative)))
}

// lineText is line n (0-based) of an open document, or of the file on disk.
func (s *server) lineText(uri string, n int) string {
	s.mu.Lock()
	text, open := s.docs[uri]
	s.mu.Unlock()
	if !open {
		data, err := os.ReadFile(uriPath(uri))
		if err != nil {
			return ""
		}
		text = string(data)
	}
	lines := strings.Split(text, "\n")
	if n < 0 || n >= len(lines) {
		return ""
	}
	return lines[n]
}

func uriPath(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return uri
	}
	return filepath.FromSlash(parsed.Path)
}

func pathURI(path string) string {
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return address.String()
}
