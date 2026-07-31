package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Messages
// ═══════════════════════════════════════════════════════════════════════════════

type lspReadyMsg struct{}
type lspErrorMsg struct{ err error }
type pollLspMsg struct{}
type clearStatusMsg struct{}
type termPollMsg struct{}
type pollCwdFileMsg struct {
	cwd string
}

type lspCompletionMsg struct {
	items []CompletionItem
	err   error
}

type lspHoverMsg struct {
	text string
	err  error
}

type lspDefinitionMsg struct {
	loc *DefinitionLocation
	err error
}

// ═══════════════════════════════════════════════════════════════════════════════
// Focus tracking
// ═══════════════════════════════════════════════════════════════════════════════

type focusPanel int

const (
	focusEditor   focusPanel = iota
	focusExplorer
	focusTerminal
)

// ═══════════════════════════════════════════════════════════════════════════════
// Editor model
// ═══════════════════════════════════════════════════════════════════════════════

// Editor is the top-level Bubble Tea model for the Cue editor.
type Editor struct {
	buffers []*Buffer
	active  int
	lsp     *LSPClient
	langID  string

	diagnostics map[string][]Diagnostic
	diagVersion int

	width  int
	height int

	statusText  string
	statusUntil time.Time

	cmdBar    bool
	cmdBuffer string

	quit          bool
	focus         focusPanel
	showExplorer  bool
	explorerWidth int
	explorer      *FileTree
	showTerminal  bool
	termHeight    int
	term          *Terminal

	cwdFilePath  string
	cwdFileLast  string

	// LSP feature state
	completionItems []CompletionItem
	completionIdx   int
	completionOpen  bool
}

func NewEditor(files []string) *Editor {
	e := &Editor{
		buffers:       []*Buffer{},
		active:        0,
		diagnostics:   make(map[string][]Diagnostic),
		explorerWidth: 28,
		termHeight:    12,
		term:          NewTerminal(),
	}

	if len(files) == 0 {
		buf, _ := NewBuffer("")
		e.buffers = append(e.buffers, buf)
	} else {
		for _, f := range files {
			buf, err := NewBuffer(f)
			if err != nil {
				buf, _ = NewBuffer("")
			}
			e.buffers = append(e.buffers, buf)
		}
	}

	// Init file explorer at CWD
	cwd, err := os.Getwd()
	if err == nil {
		e.explorer = NewFileTree(cwd)
	}

	// CUE_CWD_FILE — temp file that terminal writes cwd to, so explorer follows
	tmpDir := os.TempDir()
	e.cwdFilePath = filepath.Join(tmpDir, "cue-cwd.txt")
	e.cwdFileLast = cwd
	os.Setenv("CUE_CWD_FILE", e.cwdFilePath)
	os.WriteFile(e.cwdFilePath, []byte(cwd), 0644)

	// LSP
	e.langID = LangFromPath(e.buffers[0].Path())
	if e.langID != "" {
		e.lsp = NewLSPClient(e.langID)
		e.lsp.OnDiagnostics = func(uri string, diags []Diagnostic) {
			e.diagnostics[uri] = diags
			e.diagVersion++
		}
	}

	return e
}

func (e *Editor) Init() tea.Cmd {
	var cmds []tea.Cmd

	if e.lsp != nil {
		cmds = append(cmds, func() tea.Msg {
			if err := e.lsp.Start(); err != nil {
				return lspErrorMsg{err: err}
			}
			buf := e.buffers[e.active]
			uri := PathToURI(buf.Path())
			e.lsp.DidOpen(uri, e.langID, buf.Text())
			return lspReadyMsg{}
		})
	}

	cmds = append(cmds, e.pollLspCmd())
	cmds = append(cmds, e.termPollCmd())
	cmds = append(cmds, e.pollCwdCmd())

	return tea.Batch(cmds...)
}

// ── Poll commands ──────────────────────────────────────────────────────────

func (e *Editor) pollLspCmd() tea.Cmd {
	return func() tea.Msg {
		if e.lsp == nil {
			time.Sleep(100 * time.Millisecond)
			return pollLspMsg{}
		}
		select {
		case _, ok := <-e.lsp.MsgCh():
			if !ok {
				return pollLspMsg{}
			}
			return pollLspMsg{}
		default:
			time.Sleep(50 * time.Millisecond)
			return pollLspMsg{}
		}
	}
}

func (e *Editor) termPollCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-e.term.outputCh:
			return msg
		case err := <-e.term.doneCh:
			return termDoneMsg{err: err}
		default:
			time.Sleep(50 * time.Millisecond)
			return termPollMsg{}
		}
	}
}

func (e *Editor) pollCwdCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(e.cwdFilePath)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			return pollCwdFileMsg{}
		}
		newCwd := strings.TrimSpace(string(data))
		time.Sleep(500 * time.Millisecond)
		return pollCwdFileMsg{cwd: newCwd}
	}
}

// ── Buffer access ──────────────────────────────────────────────────────────

func (e *Editor) activeBuffer() *Buffer {
	if e.active < 0 || e.active >= len(e.buffers) {
		return nil
	}
	return e.buffers[e.active]
}

// ── Update ─────────────────────────────────────────────────────────────────

func (e *Editor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return e.handleResize(msg)

	case tea.KeyMsg:
		return e.handleKey(msg)

	case lspReadyMsg:
		return e, nil

	case lspErrorMsg:
		e.setStatus("LSP: " + msg.err.Error())
		return e, nil

	case pollLspMsg:
		return e, e.pollLspCmd()

	case termPollMsg:
		return e, e.termPollCmd()

	case termOutputMsg:
		e.term.HandleOutput(msg.line)
		return e, e.termPollCmd()

	case termDoneMsg:
		e.term.HandleDone(msg.err)
		return e, nil

	case pollCwdFileMsg:
		if msg.cwd != "" && msg.cwd != e.cwdFileLast {
			e.cwdFileLast = msg.cwd
			if e.explorer != nil {
				e.explorer.SetRoot(msg.cwd)
			}
		}
		return e, e.pollCwdCmd()

	case lspCompletionMsg:
		if msg.err != nil {
			e.setStatus("Completion: " + msg.err.Error())
		} else if len(msg.items) > 0 {
			e.completionItems = msg.items
			e.completionIdx = 0
			e.completionOpen = true
		} else {
			e.completionOpen = false
			e.completionItems = nil
		}
		return e, nil

	case lspHoverMsg:
		if msg.err != nil {
			e.setStatus("Hover: " + msg.err.Error())
		} else if msg.text != "" {
			e.setStatus(msg.text)
		}
		return e, nil

	case lspDefinitionMsg:
		if msg.err != nil {
			e.setStatus("Definition: " + msg.err.Error())
		} else if msg.loc != nil {
			e.jumpToDefinition(msg.loc)
		}
		return e, nil

	case clearStatusMsg:
		e.statusText = ""
		return e, nil
	}

	return e, nil
}

func (e *Editor) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	e.width = msg.Width
	e.height = msg.Height
	if e.explorer != nil {
		e.explorer.Resize(e.explorerWidth, e.explorerHeight())
	}
	if e.term != nil {
		e.term.Resize(e.termWidth(), e.termHeight)
	}
	return e, nil
}

// ── Key handling ───────────────────────────────────────────────────────────

func (e *Editor) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Global keys (always work) ─────────────────────────────────────

	switch msg.String() {
	case "ctrl+q":
		e.quit = true
		if e.lsp != nil {
			e.lsp.Stop()
		}
		return e, tea.Quit

	case "ctrl+b":
		e.showExplorer = !e.showExplorer
		if e.showExplorer {
			e.focus = focusExplorer
		} else {
			e.focus = focusEditor
		}
		return e, nil

	case "ctrl+t":
		e.showTerminal = !e.showTerminal
		if e.showTerminal {
			e.focus = focusTerminal
		} else {
			e.focus = focusEditor
		}
		return e, nil

	case "tab":
		e.cycleFocus()
		return e, nil

	case "?":
		e.showExplorer = !e.showExplorer
		return e, nil
	}

	// ── Command bar ──────────────────────────────────────────────────

	if e.cmdBar {
		return e.handleCmdBarKey(msg)
	}

	// ── Focus-routed keys ────────────────────────────────────────────

	switch e.focus {
	case focusExplorer:
		return e.handleExplorerKey(msg)
	case focusTerminal:
		return e.handleTerminalKey(msg)
	default:
		return e.handleEditorKey(msg)
	}
}

// ── Focus cycle ────────────────────────────────────────────────────────────

func (e *Editor) cycleFocus() {
	switch e.focus {
	case focusEditor:
		if e.showExplorer {
			e.focus = focusExplorer
		} else if e.showTerminal {
			e.focus = focusTerminal
		}
	case focusExplorer:
		if e.showTerminal {
			e.focus = focusTerminal
		} else {
			e.focus = focusEditor
		}
	case focusTerminal:
		e.focus = focusEditor
	}
}

// ── Command bar key handler ────────────────────────────────────────────────

func (e *Editor) handleCmdBarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		cmd := strings.TrimSpace(e.cmdBuffer)
		e.cmdBar = false
		e.cmdBuffer = ""
		if cmd != "" {
			return e.handleCommand(cmd)
		}
	case "esc":
		e.cmdBar = false
		e.cmdBuffer = ""
	case "backspace":
		if len(e.cmdBuffer) > 0 {
			e.cmdBuffer = e.cmdBuffer[:len(e.cmdBuffer)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			e.cmdBuffer += string(msg.Runes)
		}
	}
	return e, nil
}

// ── Explorer key handler ───────────────────────────────────────────────────

func (e *Editor) handleExplorerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if e.explorer == nil {
		e.focus = focusEditor
		return e, nil
	}

	switch msg.String() {
	case "up", "k":
		e.explorer.MoveUp()
	case "down", "j":
		e.explorer.MoveDown()
	case "enter":
		path := e.explorer.Open()
		if path != "" {
			buf, err := NewBuffer(path)
			if err != nil {
				e.setStatus("Error: " + err.Error())
			} else {
				e.buffers = append(e.buffers, buf)
				e.active = len(e.buffers) - 1
				e.setStatus("Opened " + buf.Name())
				e.focus = focusEditor
				e.maybeSwitchLSP(path, buf)
			}
		}
	case "h", "left":
		e.explorer.Toggle() // collapse
	case "l", "right":
		e.explorer.Toggle() // expand
	case "esc":
		e.focus = focusEditor
	}

	return e, nil
}

// ── Terminal key handler ───────────────────────────────────────────────────

func (e *Editor) handleTerminalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := e.term
	if t == nil {
		e.focus = focusEditor
		return e, nil
	}

	switch msg.String() {
	case "esc":
		e.focus = focusEditor
	case "enter":
		t.Execute()
		return e, e.termPollCmd()
	case "backspace":
		t.DeleteBackward()
	case "delete":
		t.DeleteForward()
	case "left":
		t.MoveLeft()
	case "right":
		t.MoveRight()
	case "home":
		t.MoveHome()
	case "end":
		t.MoveEnd()
	case "up":
		t.HistoryPrev()
	case "down":
		t.HistoryNext()
	case "tab":
		t.InsertRune('\t')
	default:
		if len(msg.Runes) > 0 {
			for _, r := range msg.Runes {
				if r >= 32 {
					t.InsertRune(r)
				}
			}
		}
	}

	return e, nil
}

// ── Editor key handler ─────────────────────────────────────────────────────

func (e *Editor) handleEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	buf := e.activeBuffer()
	if buf == nil {
		return e, nil
	}

	// Tab switching
	switch msg.String() {
	case "alt+1":
		return e.switchTab(0)
	case "alt+2":
		return e.switchTab(1)
	case "alt+3":
		return e.switchTab(2)
	case "alt+4":
		return e.switchTab(3)
	case "alt+5":
		return e.switchTab(4)
	case "alt+6":
		return e.switchTab(5)
	case "alt+7":
		return e.switchTab(6)
	case "alt+8":
		return e.switchTab(7)
	case "alt+9":
		return e.switchTab(8)
	}

	// Editor global shortcuts
	switch msg.String() {
	case "ctrl+s":
		if buf.Path() != "" {
			if err := buf.Save(); err != nil {
				e.setStatus("Save error: " + err.Error())
			} else {
				e.setStatus("Saved " + buf.Name())
				if e.lsp != nil && e.lsp.Ready() {
					e.lsp.DidChange(PathToURI(buf.Path()), buf.Text(), 1)
				}
			}
		} else {
			e.setStatus("No file path — use :save <path>")
		}
		return e, nil

	case "ctrl+n":
		e.buffers = append(e.buffers, &Buffer{lines: []string{""}})
		e.active = len(e.buffers) - 1
		e.setStatus("New buffer")
		return e, nil

	case "ctrl+w":
		if len(e.buffers) > 1 {
			e.buffers = append(e.buffers[:e.active], e.buffers[e.active+1:]...)
			if e.active >= len(e.buffers) {
				e.active = len(e.buffers) - 1
			}
			e.setStatus("Closed tab")
		}
		return e, nil

	case "ctrl+p":
		e.cmdBar = true
		e.cmdBuffer = ""
		return e, nil
	}

	// ── LSP feature keys ─────────────────────────────────────────────
	switch msg.String() {
	case "ctrl+space", "ctrl+@":
		return e, e.requestCompletion()
	case "f1":
		return e, e.requestHover()
	case "f12":
		return e, e.requestDefinition()
	}

	// ── Completion menu keys ─────────────────────────────────────────
	if e.completionOpen && len(e.completionItems) > 0 {
		switch msg.String() {
		case "up":
			if e.completionIdx > 0 {
				e.completionIdx--
			}
			return e, nil
		case "down":
			if e.completionIdx < len(e.completionItems)-1 {
				e.completionIdx++
			}
			return e, nil
		case "enter", "tab":
			e.acceptCompletion()
			return e, nil
		case "esc":
			e.completionOpen = false
			e.completionItems = nil
			return e, nil
		}
	}

	// Editor navigation & editing
	var needLSPUpdate bool
	var needCompletion bool

	wasCompletionOpen := e.completionOpen
	if e.completionOpen {
		e.completionOpen = false
		e.completionItems = nil
	}

	switch msg.String() {
	case "up":
		buf.MoveUp()
	case "down":
		buf.MoveDown()
	case "left":
		buf.MoveLeft()
	case "right":
		buf.MoveRight()
	case "home":
		buf.MoveHome()
	case "end":
		buf.MoveEnd()
	case "ctrl+left":
		buf.MoveWordLeft()
	case "ctrl+right":
		buf.MoveWordRight()
	case "pageup":
		buf.CursorRow -= e.editorHeight() / 2
		buf.ClampCursor()
	case "pagedown":
		buf.CursorRow += e.editorHeight() / 2
		buf.ClampCursor()
	case "backspace":
		buf.DeleteBackward()
		needLSPUpdate = true
		if wasCompletionOpen {
			needCompletion = true
		}
	case "delete":
		buf.DeleteForward()
		needLSPUpdate = true
		if wasCompletionOpen {
			needCompletion = true
		}
	case "enter":
		buf.NewLine()
		needLSPUpdate = true
	case "tab":
		buf.InsertString("    ")
		needLSPUpdate = true
	default:
		if len(msg.Runes) > 0 {
			for _, r := range msg.Runes {
				if r >= 32 && r != 127 {
					buf.Insert(r)
					needLSPUpdate = true
				}
			}
			if wasCompletionOpen {
				needCompletion = true
			}
		}
	}

	buf.ClampCursor()
	e.adjustScroll()

	if needLSPUpdate && e.lsp != nil && e.lsp.Ready() && buf.Path() != "" {
		e.lsp.DidChange(PathToURI(buf.Path()), buf.Text(), 1)
	}

	if needCompletion && e.lsp != nil && e.lsp.Ready() {
		return e, e.requestCompletion()
	}

	return e, nil
}

// ── LSP switching ──────────────────────────────────────────────────────────

func (e *Editor) maybeSwitchLSP(path string, buf *Buffer) {
	newLang := LangFromPath(path)
	if newLang != e.langID && newLang != "" {
		if e.lsp != nil {
			e.lsp.Stop()
		}
		e.langID = newLang
		e.lsp = NewLSPClient(e.langID)
		e.lsp.OnDiagnostics = func(uri string, diags []Diagnostic) {
			e.diagnostics[uri] = diags
			e.diagVersion++
		}
		go func() {
			if err := e.lsp.Start(); err == nil {
				e.lsp.DidOpen(PathToURI(path), newLang, buf.Text())
			}
		}()
	} else if e.lsp != nil && e.lsp.Ready() {
		e.lsp.DidOpen(PathToURI(path), e.langID, buf.Text())
	}
}

// ── LSP feature requests ────────────────────────────────────────────────────

func (e *Editor) requestCompletion() tea.Cmd {
	buf := e.activeBuffer()
	if e.lsp == nil || !e.lsp.Ready() || buf == nil || buf.Path() == "" {
		return nil
	}
	uri := PathToURI(buf.Path())
	line, char := buf.CursorRow, buf.CursorCol
	return func() tea.Msg {
		items, err := e.lsp.Completion(uri, line, char)
		return lspCompletionMsg{items: items, err: err}
	}
}

func (e *Editor) requestHover() tea.Cmd {
	buf := e.activeBuffer()
	if e.lsp == nil || !e.lsp.Ready() || buf == nil || buf.Path() == "" {
		return nil
	}
	uri := PathToURI(buf.Path())
	line, char := buf.CursorRow, buf.CursorCol
	return func() tea.Msg {
		text, err := e.lsp.Hover(uri, line, char)
		return lspHoverMsg{text: text, err: err}
	}
}

func (e *Editor) requestDefinition() tea.Cmd {
	buf := e.activeBuffer()
	if e.lsp == nil || !e.lsp.Ready() || buf == nil || buf.Path() == "" {
		return nil
	}
	uri := PathToURI(buf.Path())
	line, char := buf.CursorRow, buf.CursorCol
	return func() tea.Msg {
		loc, err := e.lsp.Definition(uri, line, char)
		return lspDefinitionMsg{loc: loc, err: err}
	}
}

// jumpToDefinition moves the cursor to a definition, opening the file if needed.
func (e *Editor) jumpToDefinition(loc *DefinitionLocation) {
	if loc == nil || loc.URI == "" {
		return
	}
	path := strings.TrimPrefix(loc.URI, "file://")
	if path == "" {
		return
	}
	idx := -1
	for i, b := range e.buffers {
		if filepath.Clean(b.Path()) == filepath.Clean(path) {
			idx = i
			break
		}
	}
	if idx < 0 {
		buf, err := NewBuffer(path)
		if err != nil {
			e.setStatus("Definition: " + err.Error())
			return
		}
		e.buffers = append(e.buffers, buf)
		idx = len(e.buffers) - 1
		e.maybeSwitchLSP(path, buf)
	}
	e.active = idx
	buf := e.buffers[idx]
	buf.CursorRow = loc.Range.Start.Line
	buf.CursorCol = loc.Range.Start.Character
	buf.ClampCursor()
	e.adjustScroll()
	e.completionOpen = false
	e.setStatus("Jumped to " + filepath.Base(path))
}

// acceptCompletion inserts the selected completion item at the cursor.
func (e *Editor) acceptCompletion() {
	buf := e.activeBuffer()
	if buf == nil || e.completionIdx < 0 || e.completionIdx >= len(e.completionItems) {
		e.completionOpen = false
		e.completionItems = nil
		return
	}
	item := e.completionItems[e.completionIdx]
	text := item.InsertText
	if text == "" {
		text = item.Label
	}
	// Replace the current word prefix under the cursor with the insertion.
	line := buf.Line(buf.CursorRow)
	start := buf.CursorCol
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	buf.CursorCol = start
	buf.InsertString(text)
	buf.ClampCursor()
	e.adjustScroll()
	e.completionOpen = false
	e.completionItems = nil
	if e.lsp != nil && e.lsp.Ready() && buf.Path() != "" {
		e.lsp.DidChange(PathToURI(buf.Path()), buf.Text(), 1)
	}
}

func isWordChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// ── Command handler ────────────────────────────────────────────────────────

func (e *Editor) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return e, nil
	}

	switch fields[0] {
	case "q", "quit":
		e.quit = true
		if e.lsp != nil {
			e.lsp.Stop()
		}
		return e, tea.Quit

	case "w", "save":
		buf := e.activeBuffer()
		if buf == nil {
			return e, nil
		}
		path := buf.Path()
		if len(fields) > 1 {
			path = fields[1]
		}
		if path == "" {
			e.setStatus("No path. Use :save <path>")
			return e, nil
		}
		if err := buf.SaveAs(path); err != nil {
			e.setStatus("Error: " + err.Error())
		} else {
			e.setStatus("Saved " + buf.Name())
		}

	case "wq":
		e.handleCommand("save " + strings.Join(fields[1:], " "))
		return e.handleCommand("quit")

	case "open":
		if len(fields) > 1 {
			path := fields[1]
			buf, err := NewBuffer(path)
			if err != nil {
				e.setStatus("Error: " + err.Error())
				return e, nil
			}
			e.buffers = append(e.buffers, buf)
			e.active = len(e.buffers) - 1
			e.setStatus("Opened " + buf.Name())
			e.maybeSwitchLSP(path, buf)
		} else {
			e.setStatus("Usage: :open <filepath>")
		}

	case "e", "explorer":
		e.showExplorer = !e.showExplorer
		if e.showExplorer {
			e.focus = focusExplorer
		}
		e.setStatus(fmt.Sprintf("Explorer: %v", map[bool]string{true: "shown", false: "hidden"}[e.showExplorer]))

	case "term", "terminal":
		e.showTerminal = !e.showTerminal
		if e.showTerminal {
			e.focus = focusTerminal
		}
		e.setStatus(fmt.Sprintf("Terminal: %v", map[bool]string{true: "shown", false: "hidden"}[e.showTerminal]))

	case "tree":
		if len(fields) > 1 && e.explorer != nil {
			e.explorer.SetRoot(fields[1])
			e.setStatus("Tree root: " + fields[1])
		} else {
			e.setStatus("Usage: :tree <directory>")
		}

	default:
		e.setStatus("Unknown command: " + fields[0])
	}

	return e, nil
}

// ── Tab switching ──────────────────────────────────────────────────────────

func (e *Editor) switchTab(idx int) (tea.Model, tea.Cmd) {
	if idx < len(e.buffers) {
		e.active = idx
		buf := e.activeBuffer()
		if buf != nil {
			e.langID = LangFromPath(buf.Path())
		}
	}
	return e, nil
}

// ── Scroll ─────────────────────────────────────────────────────────────────

func (e *Editor) adjustScroll() {
	buf := e.activeBuffer()
	if buf == nil {
		return
	}
	visLines := e.editorHeight()
	if visLines <= 0 {
		visLines = 20
	}
	if buf.CursorRow < buf.Offset {
		buf.Offset = buf.CursorRow
	}
	if buf.CursorRow >= buf.Offset+visLines {
		buf.Offset = buf.CursorRow - visLines + 1
	}
}

func (e *Editor) editorHeight() int {
	reserve := 2 // tab bar + status bar
	if e.cmdBar {
		reserve++
	}
	if e.showTerminal {
		reserve += e.termHeight + 1 // terminal + separator
	}
	return e.height - reserve - 2
}

func (e *Editor) explorerHeight() int {
	h := e.height - 3 // tab bar, status bar, terminal
	if e.showTerminal {
		h -= e.termHeight + 1
	}
	h = max(h, 5)
	return h
}

func (e *Editor) termWidth() int {
	w := e.width
	if e.showExplorer {
		w -= e.explorerWidth
	}
	if w < 20 {
		w = 20
	}
	return w
}

// ── Status ─────────────────────────────────────────────────────────────────

func (e *Editor) setStatus(msg string) {
	e.statusText = msg
	e.statusUntil = time.Now().Add(3 * time.Second)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Syntax highlighting
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// Syntax highlighting
// ═══════════════════════════════════════════════════════════════════════════════

var chromaStyle = func() *chroma.Style {
	s := styles.Get("catppuccin-macchiato")
	if s == nil {
		s = styles.Get("monokai")
	}
	if s == nil {
		s = styles.Fallback
	}
	return s
}()

// styleANSI returns ANSI escape codes (bold, italic, foreground) for a token
// type, or empty string if the token has no styling.
func styleANSI(t chroma.TokenType) string {
	if chromaStyle == nil {
		return ""
	}
	entry := chromaStyle.Get(t)
	if entry.IsZero() {
		return ""
	}
	var out strings.Builder
	if entry.Bold == chroma.Yes {
		out.WriteString("\033[1m")
	}
	if entry.Italic == chroma.Yes {
		out.WriteString("\033[3m")
	}
	if entry.Colour.IsSet() {
		r, g, b := entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue()
		fmt.Fprintf(&out, "\033[38;2;%d;%d;%dm", r, g, b)
	}
	return out.String()
}

func (e *Editor) highlightLine(line string, _ int) string {
	if e.langID == "" || line == "" {
		return line
	}
	lexer := lexers.Get(e.langID)
	if lexer == nil {
		return line
	}
	it, err := lexer.Tokenise(nil, line)
	if err != nil {
		return line
	}
	var sb strings.Builder
	for token := it(); token != chroma.EOF; token = it() {
		ansi := styleANSI(token.Type)
		if ansi != "" {
			sb.WriteString(ansi)
			sb.WriteString(token.Value)
			sb.WriteString("\033[0m")
		} else {
			sb.WriteString(token.Value)
		}
	}
	return sb.String()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Lipgloss styles
// ═══════════════════════════════════════════════════════════════════════════════

var (
	// ── Colors ──────────────────────────────────────────────────────────
	accent    = lipgloss.Color("#FF75B7")
	accentDim = lipgloss.Color("#B7558A")
	purple    = lipgloss.Color("#7C5CFC")
	green     = lipgloss.Color("#4AFA9A")
	orange    = lipgloss.Color("#FFB347")
	red       = lipgloss.Color("#FF5A5A")
	surface   = lipgloss.Color("#1E1E2E")
	surface2  = lipgloss.Color("#2A2A3E")
	borderCol = lipgloss.Color("#3A3A4E")
	muted     = lipgloss.Color("#6C6C80")
	textCol   = lipgloss.Color("#CDD6F4")

	// ── Tab bar ─────────────────────────────────────────────────────────
	tabActiveStyle = lipgloss.NewStyle().
			Background(accent).
			Foreground(lipgloss.Color("#000")).
			Padding(0, 2).
			Bold(true).
			MarginRight(1)

	tabInactiveStyle = lipgloss.NewStyle().
				Background(surface2).
				Foreground(lipgloss.Color("#888")).
				Padding(0, 2).
				MarginRight(1)

	// ── Line numbers ────────────────────────────────────────────────────
	lineNumStyle = lipgloss.NewStyle().
			Foreground(muted).
			PaddingRight(2).
			Width(5).
			Align(lipgloss.Right)

	lineNumActiveStyle = lipgloss.NewStyle().
				Foreground(accent).
				PaddingRight(2).
				Width(5).
				Align(lipgloss.Right)

	cursorLineStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#252535"))

	// ── Status bar ──────────────────────────────────────────────────────
	statusBarStyle = lipgloss.NewStyle().
			Background(surface2).
			Foreground(muted).
			Padding(0, 1)

	statusModeStyle = lipgloss.NewStyle().
			Background(accent).
			Foreground(lipgloss.Color("#000")).
			Padding(0, 1).
			Bold(true)

	statusInfoStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	// ── Diagnostics ─────────────────────────────────────────────────────
	diagnosticErrStyle = lipgloss.NewStyle().
				Foreground(red).
				Bold(true)

	diagnosticWarnStyle = lipgloss.NewStyle().
				Foreground(orange)

	dirtyMarkerStyle = lipgloss.NewStyle().
				Foreground(orange).
				Bold(true)

	// ── Panels ──────────────────────────────────────────────────────────
	panelBorderStyle = lipgloss.NewStyle().
				Foreground(borderCol)

	focusBorderStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	// ── Misc ────────────────────────────────────────────────────────────
	helpStyle = lipgloss.NewStyle().
			Foreground(muted)

	cursorStyle = lipgloss.NewStyle().
			Background(accent)

	titleBarStyle = lipgloss.NewStyle().
			Background(surface2).
			Foreground(accent).
			Bold(true).
			Padding(0, 2)

	separatorStyle = lipgloss.NewStyle().
			Foreground(borderCol)
)

// ═══════════════════════════════════════════════════════════════════════════════
// View
// ═══════════════════════════════════════════════════════════════════════════════

func (e *Editor) View() string {
	if e.quit {
		return "Goodbye!\n"
	}

	// ── Tab bar ─────────────────────────────────────────────────────────

	tabs := e.renderTabs()

	// ── Body (explorer + editor) ────────────────────────────────────────

	var bodyLeft, bodyRight string

	if e.showExplorer && e.explorer != nil {
		bodyLeft = e.explorer.View()
	}

	bodyRight = e.renderEditorBody()

	// Combine body
	var body string
	if e.showExplorer && e.explorer != nil {
		expH := e.explorerHeight()
		// Pad explorer to full height
		expLines := strings.Count(bodyLeft, "\n")
		for expLines < expH {
			bodyLeft += strings.Repeat(" ", e.explorerWidth) + "\n"
			expLines++
		}

		// Draw vertical separator
		sep := panelBorderStyle.Render("│")
		left := lipgloss.NewStyle().Width(e.explorerWidth).Render(bodyLeft)

		// Join with border
		var combined strings.Builder
		lLines := strings.Split(strings.TrimSuffix(left, "\n"), "\n")
		rLines := strings.Split(strings.TrimSuffix(bodyRight, "\n"), "\n")
		maxLines := max(len(lLines), len(rLines))
		for i := 0; i < maxLines; i++ {
			if i < len(lLines) {
				combined.WriteString(lLines[i])
			} else {
				combined.WriteString(strings.Repeat(" ", e.explorerWidth))
			}
			// Focus indicator on separator
			if e.focus == focusExplorer {
				combined.WriteString(focusBorderStyle.Render("│"))
			} else {
				combined.WriteString(sep)
			}
			if i < len(rLines) {
				combined.WriteString(rLines[i])
			}
			combined.WriteString("\n")
		}
		body = combined.String()
	} else {
		body = bodyRight
	}

	// ── Terminal panel ─────────────────────────────────────────────────

	var terminalView string
	if e.showTerminal && e.term != nil {
		termContent := e.term.View()
		// Add separator
		termSep := panelBorderStyle.Render("─")
		// Calculate width
		tw := e.width
		if e.showExplorer {
			tw -= e.explorerWidth + 1
		}
		sepLine := strings.Repeat(termSep, tw)

		if e.focus == focusTerminal {
			sepLine = focusBorderStyle.Render(strings.Repeat("━", tw))
			terminalView = sepLine + "\n" + termContent
		} else {
			terminalView = sepLine + "\n" + termContent
		}
	}

	// ── Status bar ─────────────────────────────────────────────────────

	statusBar := e.renderStatusBar()

	// ── Command bar ───────────────────────────────────────────────────

	var cmdBarView string
	if e.cmdBar {
		cmdBarView = statusModeStyle.Render(":") + e.cmdBuffer + "\n"
	}

	// Assemble final layout
	var out strings.Builder
	out.WriteString(tabs)
	out.WriteString(body)
	if e.showTerminal {
		out.WriteString("\n")
		out.WriteString(terminalView)
	}
	out.WriteString("\n")
	out.WriteString(statusBar)
	out.WriteString("\n")
	out.WriteString(cmdBarView)

	return out.String()
}

// ── Tab bar rendering ──────────────────────────────────────────────────────

func (e *Editor) renderTabs() string {
	var sb strings.Builder
	for i, b := range e.buffers {
		name := b.Name()
		if b.Dirty() {
			name = "● " + name
		}
		if i == e.active {
			sb.WriteString(tabActiveStyle.Render(name))
		} else {
			sb.WriteString(tabInactiveStyle.Render(name))
		}
	}
	// Bottom separator line
	sep := strings.Repeat("─", max(10, e.width-lipgloss.Width(sb.String())))
	sb.WriteString(separatorStyle.Render(sep))
	sb.WriteString("\n")
	return sb.String()
}

// ── Editor body rendering ──────────────────────────────────────────────────

func (e *Editor) renderEditorBody() string {
	buf := e.activeBuffer()
	if buf == nil {
		return ""
	}

	var sb strings.Builder

	visLines := e.editorHeight()
	endLine := buf.Offset + visLines
	if endLine > buf.NumLines() {
		endLine = buf.NumLines()
	}

	simpleNumStyle := lipgloss.NewStyle().Foreground(muted)
	simpleNumActiveStyle := lipgloss.NewStyle().Foreground(accent)

	for row := buf.Offset; row < endLine; row++ {
		isCursorLine := row == buf.CursorRow
		ln := fmt.Sprintf("%4d ", row+1)
		if isCursorLine {
			sb.WriteString(simpleNumActiveStyle.Render(ln))
		} else {
			sb.WriteString(simpleNumStyle.Render(ln))
		}

		line := buf.Line(row)
		var renderedLine string
		if line != "" {
			highlighted := e.highlightLine(line, row)
			if highlighted != "" {
				renderedLine = highlighted
			} else {
				renderedLine = line
			}
		}

		diag := e.diagnosticAtLine(row)
		markerLen := 0
		if diag != nil {
			marker := "•"
			if diag.Severity <= 1 {
				marker = diagnosticErrStyle.Render("•")
			} else {
				marker = diagnosticWarnStyle.Render("•")
			}
			renderedLine = marker + " " + renderedLine
			markerLen = 2
		}

		if isCursorLine {
			renderedLine = styleCursorLine(renderedLine, buf.CursorCol+markerLen)
		}
		sb.WriteString(renderedLine)
		sb.WriteString("\n")
	}

	remaining := visLines - (endLine - buf.Offset)
	if e.completionOpen && len(e.completionItems) > 0 {
		menu := e.renderCompletionMenu()
		sb.WriteString(menu)
		remaining -= strings.Count(menu, "\n")
	}
	for range remaining {
		sb.WriteString("~\n")
	}

	return sb.String()
}

// ═══════════════════════════════════════════════════════════════════════════
// Block cursor + completion menu rendering
// ═══════════════════════════════════════════════════════════════════════════

const (
	cursorLineBg  = "\033[48;2;37;37;53m"    // #252535
	cursorBlockFg = "\033[38;2;0;0;0m"       // black
	cursorBlockBg = "\033[48;2;255;117;183m" // accent #FF75B7
)

// styleCursorLine applies the cursor-line background to every visible
// character of an ANSI-styled line and draws a block cursor at visual column
// col. Syntax-highlight colors are preserved.
func styleCursorLine(highlighted string, col int) string {
	var out strings.Builder
	visCol := 0
	active := ""
	i := 0
	n := len(highlighted)
	for i < n {
		if highlighted[i] == '\033' {
			seq := readANSISequence(highlighted[i:])
			if seq == "\033[0m" {
				active = ""
			} else {
				active += seq
			}
			i += len(seq)
			continue
		}
		_, size := utf8.DecodeRuneInString(highlighted[i:])
		sgr := active + cursorLineBg
		if visCol == col {
			sgr += cursorBlockFg + cursorBlockBg
		}
		out.WriteString(sgr)
		out.WriteString(highlighted[i : i+size])
		if visCol == col {
			// Reset so the block's colors don't leak past this character.
			out.WriteString("\033[0m")
		}
		visCol++
		i += size
	}
	// Cursor beyond the end of the line: pad the gap, then draw a block.
	for visCol < col {
		out.WriteString(cursorLineBg)
		out.WriteString(" ")
		visCol++
	}
	if visCol == col {
		out.WriteString(cursorBlockFg)
		out.WriteString(cursorBlockBg)
		out.WriteString(" ")
	}
	// Reset at line end so the cursor-line colors don't leak into the
	// next row or panel.
	out.WriteString("\033[0m")
	return out.String()
}

// readANSISequence returns the complete escape sequence starting at s[0].
func readANSISequence(s string) string {
	if len(s) == 0 {
		return ""
	}
	if s[0] == '\033' && len(s) > 1 && s[1] == '[' {
		i := 2
		for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7E) {
			i++
		}
		if i < len(s) {
			i++
		}
		return s[:i]
	}
	return s[:1]
}

// renderCompletionMenu renders the active completion suggestions at the bottom
// of the editor body.
func (e *Editor) renderCompletionMenu() string {
	edW := e.width
	if e.showExplorer {
		edW -= e.explorerWidth + 1
	}
	if edW < 20 {
		edW = 60
	}
	show := min(8, len(e.completionItems))
	selected := lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#000")).Bold(true).PaddingLeft(1).PaddingRight(1).MaxWidth(edW)
	normal := lipgloss.NewStyle().Foreground(muted).PaddingLeft(1).PaddingRight(1).MaxWidth(edW)
	var sb strings.Builder
	for i := 0; i < show; i++ {
		it := e.completionItems[i]
		label := it.Label
		if it.Detail != "" {
			label += "  " + it.Detail
		}
		if i == e.completionIdx {
			sb.WriteString(selected.Render(label))
		} else {
			sb.WriteString(normal.Render(label))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Status bar rendering ──────────────────────────────────────────────────

func (e *Editor) renderStatusBar() string {
	buf := e.activeBuffer()
	if buf == nil {
		return ""
	}

	// Left: mode/status
	var leftInfo string
	if e.cmdBar {
		leftInfo = statusModeStyle.Render(" CMD ")
	} else if e.focus == focusExplorer {
		leftInfo = statusInfoStyle.Render(" EXPLORER ")
	} else if e.focus == focusTerminal {
		leftInfo = statusInfoStyle.Render(" TERMINAL ")
	} else if e.statusText != "" && time.Now().Before(e.statusUntil) {
		leftInfo = statusInfoStyle.Render(e.statusText)
	} else {
		diagCount := len(e.diagnostics[PathToURI(buf.Path())])
		if diagCount > 0 {
			leftInfo = statusInfoStyle.Render(fmt.Sprintf(" %d diagnostic(s) ", diagCount))
		} else if e.lsp != nil && !e.lsp.Ready() {
			leftInfo = statusInfoStyle.Render(" LSP connecting… ")
		} else {
			leftInfo = statusInfoStyle.Render(" Cue ")
		}
	}

	// Center: file name
	centerInfo := ""
	if buf.Path() != "" {
		centerInfo = buf.Name()
		if buf.Dirty() {
			centerInfo += " ●"
		}
	}

	// Right: position + lang
	rightInfo := fmt.Sprintf(" %s  %d:%d ", e.langID, buf.CursorRow+1, buf.CursorCol+1)

	leftW := lipgloss.Width(leftInfo)
	centerW := lipgloss.Width(centerInfo)
	rightW := lipgloss.Width(rightInfo)

	// Calculate padding: center is padded on both sides
	midPad := max(0, (e.width-leftW-rightW-centerW)/2)

	full := leftInfo +
		strings.Repeat(" ", max(0, midPad)) +
		centerInfo +
		strings.Repeat(" ", max(0, e.width-leftW-midPad-centerW-rightW)) +
		rightInfo

	return statusBarStyle.Render(full)
}

// ── Diagnostic lookup ─────────────────────────────────────────────────────

func (e *Editor) diagnosticAtLine(row int) *Diagnostic {
	buf := e.activeBuffer()
	if buf == nil {
		return nil
	}
	uri := PathToURI(buf.Path())
	diags, ok := e.diagnostics[uri]
	if !ok {
		return nil
	}
	for _, d := range diags {
		if d.Range.Start.Line <= row && row <= d.Range.End.Line {
			return &d
		}
	}
	return nil
}
