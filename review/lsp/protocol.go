// Package lsp serves lawgate review to editors over the Language Server
// Protocol on stdio: code lenses, hover, go to definition, diagnostics and
// the lawgate.* commands. It speaks only the part of LSP the review needs,
// with JSON-RPC written out by hand (no third-party library), so any LSP
// client works: VS Code (editors/vscode), Neovim, Zed, Helix.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
)

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes the server answers with.
const (
	codeMethodNotFound = -32601
	codeInternal       = -32603
)

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type documentID struct {
	URI string `json:"uri"`
}

type documentParams struct {
	TextDocument documentID `json:"textDocument"`
}

type documentPosition struct {
	TextDocument documentID `json:"textDocument"`
	Position     position   `json:"position"`
}

// readMessage reads one Content-Length framed message.
func readMessage(reader *bufio.Reader) (*rpcMessage, error) {
	header, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		return nil, fmt.Errorf("bad Content-Length: %w", err)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	var message rpcMessage
	if err := json.Unmarshal(body, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

// lineRange is the whole of a 1-based line, as an LSP range.
func lineRange(line int) lspRange {
	line = max(line, 1)
	start := position{Line: line - 1}
	end := position{Line: line - 1, Character: 1000}
	return lspRange{Start: start, End: end}
}

func rawID(text string) *json.RawMessage {
	id := json.RawMessage(strconv.Quote(text))
	return &id
}
