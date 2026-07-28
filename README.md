# Cue — Terminal Code Editor

Cue is a terminal-based code editor built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). It features LSP integration, syntax highlighting, a file explorer, and a built-in command runner. Cross-platform (Windows, macOS, Linux).

![screenshot]

## Features

- **Multi-tab editing** with syntax highlighting via [chroma](https://github.com/alecthomas/chroma)
- **LSP integration** — diagnostics, errors, and warnings inline (supports `gopls`, `clangd`, `pyright`, `typescript-language-server`, and more)
- **File explorer** — navigate and open files with keyboard shortcuts
- **Built-in terminal** — run shell commands without leaving the editor; automatically tracks `cd` so the file explorer follows
- **Command palette** — quick commands via `:` or `Ctrl+P`

## Install

```bash
go install cue@latest
```

Or build from source:

```bash
git clone <repo-url> cue
cd cue
go build -o cue.exe .
```

Requires Go 1.26+.

## Usage

```bash
cue <file1> [file2 ...]    # Open file(s)
cue                          # Start with an untitled buffer
```

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+B` | Toggle file explorer |
| `Ctrl+T` | Toggle terminal |
| `Tab` | Cycle focus (editor → explorer → terminal) |
| `Ctrl+Q` | Quit |

### Editor

| Key | Action |
|-----|--------|
| `↑`/`↓`/`←`/`→` | Cursor movement |
| `Home`/`End` | Line start/end |
| `Ctrl+←`/`Ctrl+→` | Word jump |
| `PgUp`/`PgDn` | Half-page scroll |
| `Enter` | New line |
| `Backspace`/`Delete` | Delete backward/forward |
| `Tab` | Insert 4 spaces |
| `Alt+1`..`Alt+9` | Switch to tab N |
| `Ctrl+S` | Save |
| `Ctrl+N` | New buffer |
| `Ctrl+W` | Close tab |
| `:` / `Ctrl+P` | Open command bar |

### File Explorer

| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `→`/`l` / `Enter` | Expand directory / open file |
| `←`/`h` | Collapse directory |
| `Esc` | Back to editor |

### Terminal

| Key | Action |
|-----|--------|
| Type | Enter command |
| `Enter` | Execute |
| `↑`/`↓` | Command history |
| `←`/`→` | Cursor movement |
| `Home`/`End` | Line start/end |
| `Backspace`/`Delete` | Edit |
| `Esc` | Back to editor |

## Commands (after `:` or `Ctrl+P`)

| Command | Action |
|---------|--------|
| `:q` / `:quit` | Quit |
| `:w` / `:save [path]` | Save |
| `:wq` | Save + quit |
| `:open <path>` | Open file in new tab |
| `:e` / `:explorer` | Toggle file explorer |
| `:term` / `:terminal` | Toggle terminal |
| `:tree <dir>` | Set file tree root |

## Layout

```
┌─[Tab bar]─────────────────────────────────────┐
│ main.go  │ utils.go                            │
├──────┬────────────────────────────────────────┤
│ 📁   │ 1 │ package main                        │
│ src/ │ 2 │ import "fmt"                       │
│ ▶    │ 3 │ func main() {                      │
│      │ 4 │     fmt.Println("hi")              │
│      │ 5 │ }                                   │
├──────┴────────────────────────────────────────┤
│ ─── terminal ──────────────────────────────── │
│ $ ls                                           │
│ main.go  editor/                               │
│ $ _                                            │
├───────────────────────────────────────────────┤
│ Cue  ─────────────────────────────  go  5:1  │
├───────────────────────────────────────────────┤
│ :command bar (if open)                         │
└───────────────────────────────────────────────┘
```

## LSP Support

Cue detects the language of opened files and starts the appropriate LSP server automatically. Servers are resolved from `PATH`.

| Language | Server |
|----------|--------|
| Go | `gopls` |
| TypeScript/TSX | `typescript-language-server` |
| JavaScript/JSX | `typescript-language-server` |
| Python | `pyright-langserver` |
| Rust | `rust-analyzer` |
| C/C++ | `clangd` |

## Shell Integration

The built-in terminal tracks `cd`, `pushd`, and `popd` commands. When the working directory changes, the file explorer updates to show the new directory. For native shell `cd` integration, source the provided scripts:

```bash
# bash/zsh
source scripts/cue-cd.sh

# PowerShell
. .\scripts\cue-cd.ps1

# Nushell
source scripts/cue-cd.nu
```

## Configuration

Cue creates a temporary file (`$TMPDIR/cue-cwd.txt`) and sets the `CUE_CWD_FILE` environment variable to its path. The terminal writes its current working directory to this file whenever the directory changes, and the editor polls it to keep the file explorer in sync.

## Project Structure

```
cue/
├── main.go              # Entry point
├── editor/
│   ├── buffer.go        # Text buffer (lines, cursor, editing, file I/O)
│   ├── editor.go        # Bubble Tea model (Update, View, panels)
│   ├── filetree.go      # File explorer tree
│   ├── lsp.go           # LSP client (JSON-RPC over stdio)
│   └── terminal.go      # Command runner (shell execution, viewport)
├── scripts/
│   ├── cue-cd.sh        # Shell cd hook for bash/zsh
│   ├── cue-cd.ps1       # cd hook for PowerShell
│   └── cue-cd.nu        # cd hook for Nushell
├── go.mod / go.sum
├── AGENTS.md
└── README.md
```

## Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| [bubbletea](https://github.com/charmbracelet/bubbletea) | v1.3.10 | TUI framework |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | v1.1.0 | Styling |
| [chroma/v2](https://github.com/alecthomas/chroma) | v2.27.0 | Syntax highlighting |
| [bubbles/viewport](https://github.com/charmbracelet/bubbles) | v1.0.0 | Scrollable terminal output |
