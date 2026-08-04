package editor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════════
// LSP types (minimal subset)
// ═══════════════════════════════════════════════════════════════════════════════

type lspMessage struct {
	jsonrpc   string
	id        int
	method    string
	params    json.RawMessage
	result    json.RawMessage
	err       *lspError
	isError   bool
	isResponse bool
	isRequest  bool
}

type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

type initializeParams struct {
	ProcessID    int                `json:"processId"`
	ClientInfo   *clientInfo        `json:"clientInfo,omitempty"`
	Capabilities clientCapabilities `json:"capabilities"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type clientCapabilities struct {
	TextDocument textDocumentClientCapabilities `json:"textDocument"`
}

type textDocumentClientCapabilities struct {
	Completion *struct{} `json:"completion,omitempty"`
	Hover      *struct{} `json:"hover,omitempty"`
	Definition *struct{} `json:"definition,omitempty"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier   `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent  `json:"contentChanges"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// PublishDiagnosticsParams is the notification payload from the server.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnostic represents a single LSP diagnostic (error, warning, etc.).
type Diagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity,omitempty"` // 1=err 2=warn 3=info 4=hint
	Message  string   `json:"message"`
	Source   string   `json:"source,omitempty"`
}

// LSPRange represents a range in a text document.
type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

// LSPPosition represents a zero-based position in a text document.
type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// ── Feature types (completion / hover / definition) ─────────────────────────

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

// CompletionItem is a suggestion returned by textDocument/completion.
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
}

type completionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     LSPPosition            `json:"position"`
}

type hoverParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     LSPPosition            `json:"position"`
}

type definitionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     LSPPosition            `json:"position"`
}

// DefinitionLocation is the location of a symbol definition.
type DefinitionLocation struct {
	URI   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

// ═══════════════════════════════════════════════════════════════════════════════
// LSP Client
// ═══════════════════════════════════════════════════════════════════════════════

// LSPClient manages a language server subprocess and JSON-RPC messaging.
// All exported methods are goroutine-safe.
type LSPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	msgCh  chan lspMessage
	done   chan struct{}

	mu      sync.Mutex
	closed  bool
	nextID  int
	langID  string
	ready   bool

	pending   map[int]chan lspMessage
	pendingMu sync.Mutex

	OnDiagnostics func(uri string, diags []Diagnostic)
}

// ServerForLang returns the command and args for the given language ID.
func ServerForLang(langID string) (string, []string) {
	switch langID {
	case "go":
		return "gopls", []string{}
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return "typescript-language-server", []string{"--stdio"}
	case "python":
		return "pyright-langserver", []string{"--stdio"}
	case "rust":
		return "rust-analyzer", []string{}
	case "c", "cpp":
		return "clangd", []string{}
	default:
		return "", nil
	}
}

// LangFromPath maps a file extension to an LSP language ID.
func LangFromPath(path string) string {
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i:]
			break
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc", ".cxx":
		return "cpp"
	default:
		return ""
	}
}

func NewLSPClient(langID string) *LSPClient {
	return &LSPClient{
		msgCh:   make(chan lspMessage, 64),
		done:    make(chan struct{}),
		langID:  langID,
		pending: make(map[int]chan lspMessage),
	}
}

// Start launches the LSP server and runs the initialize handshake.
func (c *LSPClient) Start() error {
	cmdName, args := ServerForLang(c.langID)
	if cmdName == "" {
		return fmt.Errorf("no LSP server for %s", c.langID)
	}

	c.cmd = exec.Command(cmdName, args...)

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmdName, err)
	}

	c.reader = bufio.NewReader(stdout)
	go c.readLoop()

	// Initialize handshake
	resp, err := c.sendRequest("initialize", initializeParams{
		ProcessID: -1,
		ClientInfo: &clientInfo{Name: "Cue", Version: "0.2.0"},
		Capabilities: clientCapabilities{
			TextDocument: textDocumentClientCapabilities{
				Completion: &struct{}{},
				Hover:      &struct{}{},
				Definition: &struct{}{},
			},
		},
	})
	if err != nil {
		c.Stop()
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.err != nil {
		c.Stop()
		return fmt.Errorf("initialize error: %s", resp.err.Message)
	}

	c.sendNotification("initialized", struct{}{})
	c.ready = true
	return nil
}

// Stop shuts down the LSP server and cleans up.
func (c *LSPClient) Stop() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	if c.ready {
		// Release lock before RPC — writeEnvelope also acquires it.
		c.sendNotification("exit", nil)
		go func() {
			c.sendRequest("shutdown", nil)
		}()
	}

	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	close(c.done)
}

func (c *LSPClient) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ready
}

// MsgCh returns the incoming LSP message channel.
func (c *LSPClient) MsgCh() <-chan lspMessage { return c.msgCh }

// DidOpen notifies the server that a document was opened.
func (c *LSPClient) DidOpen(uri, langID, text string) {
	c.sendNotification("textDocument/didOpen", didOpenParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: langID, Version: 1, Text: text,
		},
	})
}

// DidChange notifies the server of document changes.
func (c *LSPClient) DidChange(uri, text string, version int) {
	c.sendNotification("textDocument/didChange", didChangeParams{
		TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []textDocumentContentChangeEvent{{Text: text}},
	})
}

// ── Feature requests ────────────────────────────────────────────────────────

// Completion requests completion items at the given position.
func (c *LSPClient) Completion(uri string, line, char int) ([]CompletionItem, error) {
	resp, err := c.sendRequest("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     LSPPosition{Line: line, Character: char},
	})
	if err != nil {
		return nil, err
	}
	if resp.err != nil {
		return nil, fmt.Errorf("completion: %s", resp.err.Message)
	}
	// Result is either a CompletionList ({items:[...]}) or a bare array.
	var list struct {
		Items []CompletionItem `json:"items"`
	}
	if err := json.Unmarshal(resp.result, &list); err == nil && list.Items != nil {
		return list.Items, nil
	}
	var items []CompletionItem
	if err := json.Unmarshal(resp.result, &items); err != nil {
		return nil, fmt.Errorf("parse completion: %w", err)
	}
	return items, nil
}

// Hover requests hover information at the given position.
func (c *LSPClient) Hover(uri string, line, char int) (string, error) {
	resp, err := c.sendRequest("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     LSPPosition{Line: line, Character: char},
	})
	if err != nil {
		return "", err
	}
	if resp.err != nil {
		return "", fmt.Errorf("hover: %s", resp.err.Message)
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(resp.result, &h); err != nil {
		return "", fmt.Errorf("parse hover: %w", err)
	}
	return parseHoverText(h.Contents), nil
}

// Definition requests the definition location at the given position.
func (c *LSPClient) Definition(uri string, line, char int) (*DefinitionLocation, error) {
	resp, err := c.sendRequest("textDocument/definition", definitionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     LSPPosition{Line: line, Character: char},
	})
	if err != nil {
		return nil, err
	}
	if resp.err != nil {
		return nil, fmt.Errorf("definition: %s", resp.err.Message)
	}
	// Result can be a Location, []Location, or []LocationLink (targetUri).
	var links []struct {
		TargetURI   string   `json:"targetUri"`
		TargetRange LSPRange `json:"targetRange"`
	}
	if err := json.Unmarshal(resp.result, &links); err == nil && len(links) > 0 && links[0].TargetURI != "" {
		return &DefinitionLocation{URI: links[0].TargetURI, Range: links[0].TargetRange}, nil
	}
	var locs []DefinitionLocation
	if err := json.Unmarshal(resp.result, &locs); err == nil && len(locs) > 0 {
		return &locs[0], nil
	}
	var loc DefinitionLocation
	if err := json.Unmarshal(resp.result, &loc); err != nil {
		return nil, fmt.Errorf("parse definition: %w", err)
	}
	if loc.URI == "" {
		return nil, fmt.Errorf("no definition found")
	}
	return &loc, nil
}

// parseHoverText extracts readable text from LSP hover contents, which can be
// a plain string, a MarkupContent object, or an array of either.
func parseHoverText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Value != "" {
		return obj.Value
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		var parts []string
		for _, item := range arr {
			if p := parseHoverText(item); p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ── JSON-RPC I/O ───────────────────────────────────────────────────────────

func (c *LSPClient) sendRequest(method string, params any) (lspMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	body, err := json.Marshal(params)
	if err != nil {
		return lspMessage{}, fmt.Errorf("marshal: %w", err)
	}

	if err := c.writeEnvelope(jsonrpcEnvelope{
		JSONRPC: "2.0", ID: &id, Method: method, Params: body,
	}); err != nil {
		return lspMessage{}, err
	}

	// Register pending response channel
	respCh := make(chan lspMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	// Wait for response with timeout
	select {
	case resp := <-respCh:
		return resp, nil
	case <-c.done:
		return lspMessage{}, fmt.Errorf("LSP connection closed")
	case <-time.After(5 * time.Second):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return lspMessage{}, fmt.Errorf("LSP request %s timed out", method)
	}
}

func (c *LSPClient) sendNotification(method string, params any) {
	body, err := json.Marshal(params)
	if err != nil {
		return
	}
	c.writeEnvelope(jsonrpcEnvelope{
		JSONRPC: "2.0", Method: method, Params: body,
	})
}

func (c *LSPClient) writeEnvelope(env jsonrpcEnvelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}
	return nil
}

func (c *LSPClient) readLoop() {
	for {
		msg, err := c.readMessage()
		if err != nil {
			close(c.msgCh)
			return
		}
		// Route response messages to pending requesters
		if msg.isResponse {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.id]
			if ok {
				delete(c.pending, msg.id)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
				continue
			}
		}
		// Push everything else (notifications, server requests) to the channel
		c.msgCh <- msg
	}
}

func (c *LSPClient) readMessage() (lspMessage, error) {
	line, err := ReadLine(c.reader)
	if err != nil {
		return lspMessage{}, err
	}

	var contentLength int
	if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength); err != nil {
		return lspMessage{}, fmt.Errorf("bad header: %s", line)
	}

	// Read blank line
	if _, err := ReadLine(c.reader); err != nil {
		return lspMessage{}, err
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return lspMessage{}, fmt.Errorf("read body: %w", err)
	}

	var env jsonrpcEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return lspMessage{}, fmt.Errorf("unmarshal: %w", err)
	}

	msg := lspMessage{jsonrpc: env.JSONRPC}
	if env.Error != nil {
		msg.err = env.Error
		msg.isError = true
	}
	if env.ID != nil {
		msg.id = *env.ID
		if env.Method != "" {
			msg.isRequest = true
			msg.method = env.Method
			msg.params = env.Params
		} else {
			msg.isResponse = true
			msg.result = env.Result
		}
		return msg, nil
	}

	// Notification (no ID)
	msg.method = env.Method
	msg.params = env.Params

	// Handle inline: publishDiagnostics
	if env.Method == "textDocument/publishDiagnostics" && c.OnDiagnostics != nil {
		var dp PublishDiagnosticsParams
		if json.Unmarshal(env.Params, &dp) == nil {
			c.OnDiagnostics(dp.URI, dp.Diagnostics)
		}
	}

	return msg, nil
}

// PathToURI converts a file path to an LSP file:// URI.
func PathToURI(path string) string {
	return "file://" + path
}
