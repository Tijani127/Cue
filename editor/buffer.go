package editor

import (
	"bufio"
	"bytes"
	"os"
	"strings"
)

// Buffer manages a single text buffer — a list of lines with cursor state.
type Buffer struct {
	lines   []string
	path    string
	dirty   bool

	CursorRow int
	CursorCol int

	// Scroll offset for viewport
	Offset int
}

func NewBuffer(path string) (*Buffer, error) {
	b := &Buffer{
		lines: []string{""},
		path:  path,
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return b, nil
			}
			return nil, err
		}
		// Normalize line endings and split
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		content = strings.ReplaceAll(content, "\r", "\n")
		b.lines = strings.Split(content, "\n")
		// Remove trailing empty line that split adds for files ending in \n
		if len(b.lines) > 1 && b.lines[len(b.lines)-1] == "" {
			b.lines = b.lines[:len(b.lines)-1]
		}
	}
	if len(b.lines) == 0 {
		b.lines = []string{""}
	}
	return b, nil
}

// ── Accessors ──────────────────────────────────────────────────────────────

func (b *Buffer) Name() string {
	if b.path == "" {
		return "untitled"
	}
	parts := strings.Split(b.path, string(os.PathSeparator))
	return parts[len(parts)-1]
}

func (b *Buffer) Path() string  { return b.path }
func (b *Buffer) Dirty() bool   { return b.dirty }
func (b *Buffer) Lines() []string { return b.lines }
func (b *Buffer) NumLines() int  { return len(b.lines) }

// Line returns the text at the given row (0-indexed). Returns empty string for
// out-of-range.
func (b *Buffer) Line(row int) string {
	if row < 0 || row >= len(b.lines) {
		return ""
	}
	return b.lines[row]
}

// ── Cursor ─────────────────────────────────────────────────────────────────

func (b *Buffer) ClampCursor() {
	if b.CursorRow < 0 {
		b.CursorRow = 0
	}
	if b.CursorRow >= len(b.lines) {
		b.CursorRow = len(b.lines) - 1
	}
	lineLen := len(b.lines[b.CursorRow])
	if b.CursorCol < 0 {
		b.CursorCol = 0
	}
	if b.CursorCol > lineLen {
		b.CursorCol = lineLen
	}
}

// ── Editing operations ─────────────────────────────────────────────────────

func (b *Buffer) Insert(ch rune) {
	b.ClampCursor()
	line := b.lines[b.CursorRow]
	start := line[:b.CursorCol]
	end := line[b.CursorCol:]
	b.lines[b.CursorRow] = start + string(ch) + end
	b.CursorCol++
	b.dirty = true
}

func (b *Buffer) InsertString(s string) {
	b.ClampCursor()
	line := b.lines[b.CursorRow]
	start := line[:b.CursorCol]
	end := line[b.CursorCol:]
	b.lines[b.CursorRow] = start + s + end
	b.CursorCol += len(s)
	b.dirty = true
}

func (b *Buffer) NewLine() {
	b.ClampCursor()
	line := b.lines[b.CursorRow]
	rest := line[b.CursorCol:]
	b.lines[b.CursorRow] = line[:b.CursorCol]
	// Insert new line after current
	b.lines = append(b.lines[:b.CursorRow+1], append([]string{rest}, b.lines[b.CursorRow+1:]...)...)
	b.CursorRow++
	b.CursorCol = 0
	b.dirty = true
}

func (b *Buffer) DeleteBackward() bool {
	b.ClampCursor()
	if b.CursorCol > 0 {
		line := b.lines[b.CursorRow]
		b.lines[b.CursorRow] = line[:b.CursorCol-1] + line[b.CursorCol:]
		b.CursorCol--
		b.dirty = true
		return true
	}
	if b.CursorRow > 0 {
		prevLen := len(b.lines[b.CursorRow-1])
		b.lines[b.CursorRow-1] += b.lines[b.CursorRow]
		b.lines = append(b.lines[:b.CursorRow], b.lines[b.CursorRow+1:]...)
		b.CursorRow--
		b.CursorCol = prevLen
		b.dirty = true
		return true
	}
	return false
}

func (b *Buffer) DeleteForward() bool {
	b.ClampCursor()
	line := b.lines[b.CursorRow]
	if b.CursorCol < len(line) {
		b.lines[b.CursorRow] = line[:b.CursorCol] + line[b.CursorCol+1:]
		b.dirty = true
		return true
	}
	if b.CursorRow < len(b.lines)-1 {
		b.lines[b.CursorRow] += b.lines[b.CursorRow+1]
		b.lines = append(b.lines[:b.CursorRow+1], b.lines[b.CursorRow+2:]...)
		b.dirty = true
		return true
	}
	return false
}

func (b *Buffer) DeleteLine() {
	b.ClampCursor()
	if len(b.lines) == 1 {
		b.lines[0] = ""
		b.CursorCol = 0
		b.dirty = true
		return
	}
	b.lines = append(b.lines[:b.CursorRow], b.lines[b.CursorRow+1:]...)
	if b.CursorRow >= len(b.lines) {
		b.CursorRow--
	}
	b.CursorCol = 0
	b.dirty = true
}

// ── Cursor movement ────────────────────────────────────────────────────────

func (b *Buffer) MoveUp()    { b.CursorRow--; b.ClampCursor() }
func (b *Buffer) MoveDown()  { b.CursorRow++; b.ClampCursor() }

func (b *Buffer) MoveLeft() {
	if b.CursorCol > 0 {
		b.CursorCol--
	} else if b.CursorRow > 0 {
		b.CursorRow--
		b.CursorCol = len(b.lines[b.CursorRow])
	}
}

func (b *Buffer) MoveRight() {
	lineLen := len(b.lines[b.CursorRow])
	if b.CursorCol < lineLen {
		b.CursorCol++
	} else if b.CursorRow < len(b.lines)-1 {
		b.CursorRow++
		b.CursorCol = 0
	}
}

func (b *Buffer) MoveHome()     { b.CursorCol = 0 }
func (b *Buffer) MoveEnd()      { b.CursorCol = len(b.lines[b.CursorRow]) }

func (b *Buffer) MoveWordLeft() {
	if b.CursorCol == 0 && b.CursorRow > 0 {
		b.CursorRow--
		b.CursorCol = len(b.lines[b.CursorRow])
		return
	}
	line := b.lines[b.CursorRow]
	if b.CursorCol > 0 {
		pos := b.CursorCol - 1
		// Skip whitespace
		for pos > 0 && (line[pos] == ' ' || line[pos] == '\t') {
			pos--
		}
		// Skip word characters
		for pos > 0 && line[pos] != ' ' && line[pos] != '\t' {
			pos--
		}
		if pos == 0 && line[pos] != ' ' && line[pos] != '\t' {
			b.CursorCol = 0
		} else {
			b.CursorCol = pos + 1
		}
	}
}

func (b *Buffer) MoveWordRight() {
	line := b.lines[b.CursorRow]
	if b.CursorCol >= len(line) && b.CursorRow < len(b.lines)-1 {
		b.CursorRow++
		b.CursorCol = 0
		return
	}
	pos := b.CursorCol
	// Skip word characters
	for pos < len(line) && line[pos] != ' ' && line[pos] != '\t' {
		pos++
	}
	// Skip whitespace
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	b.CursorCol = pos
}

// ── File I/O ───────────────────────────────────────────────────────────────

func (b *Buffer) Save() error {
	if b.path == "" {
		return nil // call SaveAs instead
	}
	return b.writeFile()
}

func (b *Buffer) SaveAs(path string) error {
	b.path = path
	return b.writeFile()
}

func (b *Buffer) writeFile() error {
	var buf bytes.Buffer
	for i, line := range b.lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	buf.WriteByte('\n')
	if err := os.WriteFile(b.path, buf.Bytes(), 0644); err != nil {
		return err
	}
	b.dirty = false
	return nil
}

// ── Content ────────────────────────────────────────────────────────────────

func (b *Buffer) Text() string {
	return strings.Join(b.lines, "\n")
}

// RuneAt returns the rune at the given position, or 0 if out of range.
func (b *Buffer) RuneAt(row, col int) rune {
	line := b.Line(row)
	if col >= len(line) {
		return 0
	}
	return []rune(line)[col]
}

// ── Readline helper for external use ───────────────────────────────────────

// ReadLine reads a line of text without the terminating newline.
// Used internally by the LSP client for reading line-delimited content.
func ReadLine(r *bufio.Reader) (string, error) {
	var buf strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			return buf.String(), err
		}
		if b == '\n' {
			// Strip carriage return if present
			s := buf.String()
			if len(s) > 0 && s[len(s)-1] == '\r' {
				return s[:len(s)-1], nil
			}
			return s, nil
		}
		buf.WriteByte(b)
	}
}
