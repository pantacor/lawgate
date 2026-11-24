package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pantacor/lawgate/review"
	"github.com/pantacor/lawgate/review/internal/fixture"
)

// client drives the server over a pipe, as an editor does.
type client struct {
	t   *testing.T
	in  io.Writer
	out *bufio.Reader
	id  int
	// notifications are the server's notifications, by method, seen while
	// waiting for answers.
	notifications map[string][]json.RawMessage
}

func (c *client) send(method string, params any, id int) {
	message := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id > 0 {
		message["id"] = id
	}
	data, _ := json.Marshal(message)
	fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

// call sends a request and returns its result, keeping notifications.
func (c *client) call(method string, params any) json.RawMessage {
	c.t.Helper()
	c.id++
	c.send(method, params, c.id)
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		message, err := readMessage(c.out)
		if err != nil {
			c.t.Fatalf("%s: %v", method, err)
		}
		if result, done := c.answer(method, message); done {
			return result
		}
	}
	c.t.Fatalf("%s: no answer", method)
	return nil
}

// waitFor reads until the server sends a notification of method.
func (c *client) waitFor(method string) json.RawMessage {
	c.t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		message, err := readMessage(c.out)
		if err != nil {
			c.t.Fatalf("waiting for %s: %v", method, err)
		}
		if message.Method == method {
			return message.Params
		}
	}
	c.t.Fatalf("no %s", method)
	return nil
}

func (c *client) answer(method string, message *rpcMessage) (json.RawMessage, bool) {
	if message.Method != "" {
		c.notifications[message.Method] = append(c.notifications[message.Method], message.Params)
		return nil, false
	}
	if !answers(message, c.id) {
		return nil, false
	}
	if message.Error != nil {
		c.t.Fatalf("%s: %s", method, message.Error.Message)
	}
	result, _ := json.Marshal(message.Result)
	return result, true
}

// answers reports whether message is the response to request id.
func answers(message *rpcMessage, id int) bool {
	var got int
	if message.ID == nil || json.Unmarshal(*message.ID, &got) != nil {
		return false
	}
	return got == id
}

func startServer(t *testing.T, root string) *client {
	t.Helper()
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	build := func(root string) (*review.Index, error) { return review.Build(root, review.Options{}) }
	go func() {
		_ = Serve(Config{Build: build}, inReader, outWriter)
		outWriter.Close()
	}()
	t.Cleanup(func() { inWriter.Close() })
	c := &client{t: t, in: inWriter, out: bufio.NewReader(outReader), notifications: make(map[string][]json.RawMessage)}
	c.call("initialize", map[string]any{"rootUri": pathURI(root)})
	c.send("initialized", map[string]any{}, 0)
	return c
}

func at(uri string, line, character int) map[string]any {
	document := map[string]any{"uri": uri}
	where := map[string]any{"line": line, "character": character}
	return map[string]any{"textDocument": document, "position": where}
}

// TestLanguageServer drives initialize, code lenses, hover, definition and
// the lawgate.* commands over a pipe on an adopting repository.
func TestLanguageServer(t *testing.T) {
	root := fixture.Repo(t)
	c := startServer(t, root)
	worker := pathURI(filepath.Join(root, "worker.go"))
	tagLine := fixture.Line(t, root, "worker.go", `"SHOP_JOB 1`) - 1

	checkLenses(t, c, root, worker)
	hover := string(c.call("textDocument/hover", at(worker, tagLine, 20)))
	for _, want := range []string{"Site `1`", "state `pay`", "never-overspend", "Why this line changed"} {
		if !strings.Contains(hover, want) {
			t.Errorf("hover lacks %q:\n%s", want, hover)
		}
	}
	var definitions []location
	if err := json.Unmarshal(c.call("textDocument/definition", at(worker, tagLine, 20)), &definitions); err != nil {
		t.Fatal(err)
	}
	declLine := fixture.Line(t, root, "machine.go", `Tag: "1"`) - 1
	jumps := len(definitions) == 1 && strings.HasSuffix(definitions[0].URI, "/machine.go")
	if !jumps || definitions[0].Range.Start.Line != declLine {
		t.Fatalf("definition = %+v, want machine.go:%d", definitions, declLine+1)
	}
	checkLawHeading(t, c, root)
	checkCommands(t, c, worker, tagLine)
	checkReindexOnSave(t, c, root, worker)
	c.call("shutdown", nil)
}

// checkReindexOnSave saves worker.go with a new site and waits for the
// server to announce an index that has it.
func checkReindexOnSave(t *testing.T, c *client, root, worker string) {
	t.Helper()
	fixture.Replace(t, root, "worker.go", "func record(tag string) error {\n\treturn nil", "func record(tag string) error {\n\tfail(\"SHOP_JOB 3 unclaimed\")\n\treturn nil")
	saved := documentID{URI: worker}
	c.send("textDocument/didSave", documentParams{TextDocument: saved}, 0)
	var summary review.Summary
	if err := json.Unmarshal(c.waitFor("lawgate/indexChanged"), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Drift != 1 {
		t.Fatalf("summary after the save = %+v, want the unclaimed tag as drift", summary)
	}
}

func checkLenses(t *testing.T, c *client, root, worker string) {
	t.Helper()
	var lenses []codeLens
	document := documentID{URI: worker}
	params := documentParams{TextDocument: document}
	if err := json.Unmarshal(c.call("textDocument/codeLens", params), &lenses); err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, found := range lenses {
		titles = append(titles, found.Command.Title)
	}
	all := strings.Join(titles, "\n")
	wants := []string{"⚙ state pay/pay · 2 sites · ✅ no-silent-errors", "◆ fail 1 · pay/charge", "🛡 protected by law never-overspend"}
	for _, want := range wants {
		if !strings.Contains(all, want) {
			t.Errorf("lenses lack %q:\n%s", want, all)
		}
	}
}

// checkLawHeading hovers a LAWS.md heading and jumps from its test to the
// test function.
func checkLawHeading(t *testing.T, c *client, root string) {
	t.Helper()
	laws := pathURI(filepath.Join(root, "LAWS.md"))
	heading := fixture.Line(t, root, "LAWS.md", "## no-silent-errors") - 1
	if hover := string(c.call("textDocument/hover", at(laws, heading, 5))); !strings.Contains(hover, "holds on `pay`") {
		t.Errorf("law hover:\n%s", hover)
	}
	enforced := fixture.Line(t, root, "LAWS.md", "TestRefusesOverBalance") - 1
	var definitions []location
	if err := json.Unmarshal(c.call("textDocument/definition", at(laws, enforced, 30)), &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || !strings.HasSuffix(definitions[0].URI, "/worker_test.go") {
		t.Fatalf("definition from the enforcer = %+v", definitions)
	}
}

func checkCommands(t *testing.T, c *client, worker string, tagLine int) {
	t.Helper()
	var summary review.Summary
	if err := json.Unmarshal(c.call("workspace/executeCommand", map[string]any{"command": CommandSummary}), &summary); err != nil {
		t.Fatal(err)
	}
	counted := summary.Holds == 1 && summary.Sites == 2
	if !counted || summary.Drift != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	var why review.Commit
	whyArgs := map[string]any{"command": CommandWhy, "arguments": []any{worker, tagLine}}
	if err := json.Unmarshal(c.call("workspace/executeCommand", whyArgs), &why); err != nil || why.Subject != "shop: pay within the balance" {
		t.Fatalf("why = %+v (%v)", why, err)
	}
	var markdown string
	reviewArgs := map[string]any{"command": CommandReviewMarkdown, "arguments": []any{"main"}}
	if err := json.Unmarshal(c.call("workspace/executeCommand", reviewArgs), &markdown); err != nil || !strings.HasPrefix(markdown, review.ReportMarker) {
		t.Fatalf("markdown = %q (%v)", markdown, err)
	}
	var reindexed review.Summary
	if err := json.Unmarshal(c.call("workspace/executeCommand", map[string]any{"command": CommandReindex}), &reindexed); err != nil || reindexed != summary {
		t.Fatalf("reindex = %+v (%v), want %+v", reindexed, err, summary)
	}
	if len(c.notifications["lawgate/indexChanged"]) == 0 {
		t.Fatal("the server did not announce the new index")
	}
}
