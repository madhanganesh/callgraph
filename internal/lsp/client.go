package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- JSON-RPC types ---

type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Client ---

// Client is a JSON-RPC client that communicates with gopls over stdin/stdout.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	nextID atomic.Int64

	writeMu sync.Mutex // protects stdin writes

	pendingMu sync.Mutex
	pending   map[int]chan *jsonrpcMessage

	done chan struct{} // closed when readLoop exits
}

// NewClient starts an LSP server and performs the initialize handshake.
// lspCmd is the command to run (e.g. ["gopls", "serve"]).
func NewClient(rootDir string, lspCmd []string) (*Client, error) {
	cmd := exec.Command(lspCmd[0], lspCmd[1:]...)
	cmd.Dir = rootDir
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", lspCmd[0], err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewReaderSize(stdout, 1<<16),
		pending: make(map[int]chan *jsonrpcMessage),
		done:    make(chan struct{}),
	}

	go c.readLoop()

	if err := c.initialize(rootDir); err != nil {
		c.Close()
		return nil, fmt.Errorf("LSP initialize: %w", err)
	}

	return c, nil
}

// --- Public LSP methods ---

// DidOpen notifies the language server that a file has been opened.
func (c *Client) DidOpen(uri, languageID, text string) error {
	return c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       text,
		},
	})
}

// PrepareCallHierarchy returns the call hierarchy item(s) at the given position.
// Position is 0-based (LSP convention).
func (c *Client) PrepareCallHierarchy(uri string, line, col int) ([]CallHierarchyItem, error) {
	result, err := c.call("textDocument/prepareCallHierarchy", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	})
	if err != nil {
		return nil, err
	}

	var items []CallHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, fmt.Errorf("unmarshal prepareCallHierarchy: %w", err)
	}
	return items, nil
}

// IncomingCalls returns the callers of the given call hierarchy item.
func (c *Client) IncomingCalls(item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	result, err := c.call("callHierarchy/incomingCalls", CallHierarchyIncomingCallsParams{
		Item: item,
	})
	if err != nil {
		return nil, err
	}

	var calls []CallHierarchyIncomingCall
	if err := json.Unmarshal(result, &calls); err != nil {
		return nil, fmt.Errorf("unmarshal incomingCalls: %w", err)
	}
	return calls, nil
}

// OutgoingCalls returns the callees (functions called by) the given item.
func (c *Client) OutgoingCalls(item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	result, err := c.call("callHierarchy/outgoingCalls", CallHierarchyOutgoingCallsParams{
		Item: item,
	})
	if err != nil {
		return nil, err
	}

	var calls []CallHierarchyOutgoingCall
	if err := json.Unmarshal(result, &calls); err != nil {
		return nil, fmt.Errorf("unmarshal outgoingCalls: %w", err)
	}
	return calls, nil
}

// Implementation returns the implementation locations for the symbol at the
// given position. Used to detect interface methods and offer a picker.
// Position is 0-based.
func (c *Client) Implementation(uri string, line, col int) ([]Location, error) {
	result, err := c.call("textDocument/implementation", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	// Server may return Location | Location[] | LocationLink[]. Try array first.
	var locs []Location
	if err := json.Unmarshal(result, &locs); err == nil {
		return locs, nil
	}
	var single Location
	if err := json.Unmarshal(result, &single); err == nil {
		return []Location{single}, nil
	}
	return nil, fmt.Errorf("unmarshal implementation: unexpected shape")
}

// Close performs a graceful LSP shutdown and kills gopls.
func (c *Client) Close() {
	// Best-effort shutdown with a timeout so we don't hang.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		c.call("shutdown", nil)
		c.notify("exit", nil)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
	}

	c.stdin.Close()
	<-c.done
	c.cmd.Wait()
}

// --- Internals ---

func (c *Client) initialize(rootDir string) error {
	uri := FileURI(rootDir)
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   uri,
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				CallHierarchy: CallHierarchyClientCapabilities{
					DynamicRegistration: true,
				},
			},
		},
		WorkspaceFolders: []WorkspaceFolder{
			{URI: uri, Name: rootDir},
		},
	}

	if _, err := c.call("initialize", params); err != nil {
		return err
	}

	return c.notify("initialized", struct{}{})
}

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = b
	}

	ch := make(chan *jsonrpcMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	msg := &jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsRaw,
	}

	if err := c.writeMessage(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("lsp %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-c.done:
		return nil, fmt.Errorf("connection closed while waiting for %s", method)
	case <-time.After(60 * time.Second):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("timeout waiting for %s response", method)
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = b
	}

	msg := &jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}

	return c.writeMessage(msg)
}

// respondToServer sends a response to a server-initiated request.
func (c *Client) respondToServer(id int, result any) {
	var raw json.RawMessage
	if result != nil {
		b, _ := json.Marshal(result)
		raw = b
	} else {
		raw = json.RawMessage("null")
	}

	msg := &jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  raw,
	}

	c.writeMessage(msg)
}

// --- Message framing (LSP Content-Length protocol) ---

func (c *Client) writeMessage(msg *jsonrpcMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

func (c *Client) readMessage() (*jsonrpcMessage, error) {
	var contentLength int
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLength = n
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	return &msg, nil
}

// readLoop continuously reads messages from gopls and dispatches them.
func (c *Client) readLoop() {
	defer close(c.done)

	for {
		msg, err := c.readMessage()
		if err != nil {
			return
		}

		if msg.ID != nil && msg.Method == "" {
			// Response to one of our requests.
			c.pendingMu.Lock()
			ch, ok := c.pending[*msg.ID]
			if ok {
				delete(c.pending, *msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
			}
		} else if msg.ID != nil && msg.Method != "" {
			// Server-initiated request — must respond or gopls blocks.
			c.handleServerRequest(msg)
		}
		// Notifications (ID == nil) from server are silently ignored.
	}
}

// handleServerRequest auto-responds to requests from gopls.
func (c *Client) handleServerRequest(msg *jsonrpcMessage) {
	switch msg.Method {
	case "workspace/configuration":
		// gopls asks for workspace config. Return one empty object per item.
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		json.Unmarshal(msg.Params, &p)
		configs := make([]map[string]any, len(p.Items))
		for i := range configs {
			configs[i] = map[string]any{}
		}
		c.respondToServer(*msg.ID, configs)
	default:
		// window/workDoneProgress/create, client/registerCapability, etc.
		c.respondToServer(*msg.ID, nil)
	}
}
