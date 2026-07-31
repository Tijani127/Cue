# AGENTS.md — Cue

A **terminal code editor** built with Bubble Tea, featuring **LSP integration**, **chroma syntax highlighting**, a **file explorer**, and a **command runner**. Cross-platform (Windows, macOS, Linux).

## Quick start

```
cue <file1> [file2 ...]    # Open files
cue                          # New untitled buffer
```

## Commands

| Action | Command |
|--------|---------|
| Build | `go build -o cue.exe .` |
| Run | `go run . <file>` |
| Run (no file) | `go run .` |
| Lint | `go vet ./...` |
| Tidy | `go mod tidy` |

All commands from project root. No Makefile, no test suite.

## Project structure

```
cue/
├── main.go              # Entry point: CLI args → NewEditor → tea.NewProgram
├── editor/
│   ├── buffer.go        # Text buffer: lines, cursor, edit ops, file I/O
│   ├── lsp.go           # LSP client: JSON-RPC over stdio, lifecycle
│   ├── editor.go        # Editor model: Bubble Tea Update/View, syntax highlighting, panels
│   ├── filetree.go      # File explorer tree: navigation, directory expansion
│   └── terminal.go      # Terminal/command runner: shell execution, output viewport
├── scripts/
│   ├── cue-cd.sh        # Bash/zsh: hook cd to write $CUE_CWD_FILE
│   ├── cue-cd.ps1       # PowerShell: override Set-Location to write $env:CUE_CWD_FILE
│   └── cue-cd.nu        # Nushell: override cd to write $env.CUE_CWD_FILE
├── go.mod / go.sum
└── AGENTS.md
```

Module: **`cue`**, library subpackage: **`cue/editor`**. `main.go` imports `cue/editor`.

## Architecture

### Panel system
Three panels with focus cycling via `Tab`:

| Panel | Toggle key | Focus | Description |
|-------|-----------|-------|-------------|
| Editor (default) | — | `focusEditor` | Code editing with syntax highlighting |
| File explorer | `Ctrl+B` | `focusExplorer` | Directory tree, open files |
| Terminal | `Ctrl+T` | `focusTerminal` | Command execution with output |

The `focus` field (`focusPanel`) routes keyboard input to the active panel. Each panel has its own `handle*Key` method.

### Layout
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
│ :command bar (if active)                       │
└───────────────────────────────────────────────┘
```

Panels: left (explorer), center-right (editor), bottom (terminal with `─` separator). The status bar sits below the terminal. Focused panel shows accent-colored border.

## Files

### editor/filetree.go — File Explorer
- **`FileNode`**: name, path, isDir, expanded, depth, children
- **`FileTree`**: root node, cursor, flat `visible` list for keyboard navigation
- Sorted: directories first, then files, case-insensitive
- Hidden files (`.` prefix) are excluded by default
- `Toggle()` expands/collapses directories with lazy child loading
- `Open()` returns file path (or toggles directory)
- Keyboard: `↑/k`, `↓/j`, `h`/`left` collapse, `l`/`right` expand, `enter` open, `esc` back to editor
- `SetRoot(path)` re-roots the tree
- View renders with `▶`/`▼` prefixes for directories, indentation based on depth

### editor/terminal.go — Command Runner
Not a full terminal emulator — executes shell commands and shows output. Cross-platform via `os/exec`.

- **Shell detection**: `detectShell()` checks for pwsh > powershell > cmd (Windows), zsh > bash (Unix)
- **Input**: line-based with cursor, history (↑/↓), home/end
- **Execution**: `startCmd()` spawns shell with `-c` flag, captures stdout/stderr, streams via channels
- **Output**: capped at 5000 lines, displayed via `viewport.Model` with auto-scroll
- **Concurrent output**: goroutines read stdout/stderr and push to `outputCh`; `doneCh` signals completion
- **Polling**: editor.go polls `outputCh` / `doneCh` via `termPollCmd()`, chains recursively
- Channels have 256-buffer to avoid blocking

### editor/editor.go — Main Model
- **`Editor`** is the top-level `tea.Model`
- Routes messages to panels based on `focus`
- `Update()` dispatches to handleResize, handleKey, LSP messages, terminal messages
- `View()` assembles layout from tab bar → body (explorer + editor) → terminal → status bar → command bar
- Status bar shows status text / diagnostics count / app name on the left, `lang  Row:Col` on the right
- `adjustScroll()` keeps cursor visible in editor viewport
- `renderTabs()`, `renderEditorBody()`, `renderStatusBar()` are separate for clarity
- **CUE_CWD_FILE**: On startup creates a temp file (`$TMPDIR/cue-cwd.txt`) and sets `CUE_CWD_FILE` env var. `pollCwdCmd()` reads this file every ~500ms; if the contents differ from the last known value, it calls `explorer.SetRoot()` to keep the file explorer in sync with the terminal's working directory.

## Keybindings

### Global
| Key | Action |
|-----|--------|
| `Ctrl+B` | Toggle file explorer |
| `Ctrl+T` | Toggle terminal |
| `Tab` | Cycle focus (editor → explorer → terminal → editor) |
| `?` | Toggle help or explorer (currently toggles explorer) |
| `Ctrl+Q` | Quit |

### Editor (when focused)
| Key | Action |
|-----|--------|
| `↑`/`↓`/`←`/`→` | Cursor movement |
| `Home`/`End` | Line start/end |
| `Ctrl+←`/`Ctrl+→` | Word jump |
| `PgUp`/`PgDn` | Half-page scroll |
| `Enter` | New line |
| `Backspace`/`Delete` | Delete backward/forward |
| `Tab` | Insert 4 spaces |
| Printable chars | Insert at cursor |
| `Alt+1`..`Alt+9` | Switch tab N |
| `Ctrl+S` | Save |
| `Ctrl+N` | New buffer |
| `Ctrl+W` | Close tab |
| `:` | Command bar |
| `Ctrl+P` | Command bar (type `:q`, `:w`, `:open <path>`, etc.) |
| `Ctrl+Space` | Completion menu (↑/↓ select, Enter/Tab accept, Esc close) |
| `F1` | Hover info at cursor (status bar) |
| `F12` | Go to definition (opens file in new tab if needed) |

### File explorer (when focused)
| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `→`/`l` / `Enter` | Expand directory / open file |
| `←`/`h` | Collapse directory |
| `Esc` | Back to editor |

### Terminal (when focused)
| Key | Action |
|-----|--------|
| Type | Enter command |
| `Enter` | Execute |
| `↑`/`↓` | Command history |
| `←`/`→` | Cursor movement |
| `Home`/`End` | Line start/end |
| `Backspace`/`Delete` | Edit |
| `Esc` | Back to editor |

### Commands (after `Ctrl+P`)
| Command | Action |
|---------|--------|
| `:q` / `:quit` | Quit |
| `:w` / `:save [path]` | Save |
| `:wq` | Save + quit |
| `:open <path>` | Open file in new tab |
| `:e` / `:explorer` | Toggle file explorer |
| `:term` / `:terminal` | Toggle terminal |
| `:tree <dir>` | Set file tree root |

## Cross-platform notes

- **Shell detection** in `terminal.go` uses priority order: nu > pwsh > bash > zsh > cmd (cross-platform via `exec.LookPath`)
- **CUE_CWD_FILE**: Editor writes a temp file path to the `CUE_CWD_FILE` env var. The terminal writes its tracked `cwd` to this file on every `cd`/`pushd`/`popd`. The editor polls the file and updates the explorer root. Shell scripts in `scripts/` allow native shell `cd` to write to this file for tighter integration.
- **cd tracking**: `parseCd()` in terminal.go intercepts `cd`, `pushd`, and `popd` commands so the terminal's internal cwd stays in sync without a real PTY. `writeCwdFile()` writes the tracked cwd to `CUE_CWD_FILE` after each change.
- **File paths** use `filepath.Join` and `filepath.Base` throughout (not manual `/` or `\`)
- **LSP server names** are hardcoded — must be on `PATH` (e.g. `gopls`, `clangd`, `pyright-langserver`, `typescript-language-server`)
- **Line endings**: `NewBuffer` in buffer.go normalizes `\r\n` / `\r` to `\n`
- **Terminal viewport** uses `bubbles/viewport` which works on all platforms

## Gotchas

### StyleEntry is a struct, not a pointer
In chroma v2, `style.Get(t)` returns `chroma.StyleEntry` (struct). Use `entry.Colour.IsSet()` rather than `entry == nil`.

### .Copy() is deprecated in lipgloss
Lipgloss 1.1.0 deprecates `.Copy()` — all style methods return a new style already.

### Channel-based polling for LSP and terminal
Both LSP and terminal use channel + polling pattern. `pollLspCmd()` and `termPollCmd()` run every ~50ms, checking for new messages. After handling a message, they return a new poll command to chain. If no message is available, they return a `pollMsg` after a short sleep to re-queue.

### Terminal is not a PTY
The terminal runs commands via `exec.Command(shell, "-c", cmd)`. It captures stdout/stderr as complete lines. Interactive programs (less, vim, top) won't work. Use for build commands, git, file operations, and scripts.

### LSP lifecycle per-language
When opening a file of a different language, the old LSP server is stopped and a new one started. `maybeSwitchLSP()` handles this. The LSP starts in a goroutine to avoid blocking the UI.

### Explorer uses flat visible list
`FileTree.flatten()` creates a linear `visible` slice for keyboard navigation. Expanding/collapsing a directory rebuilds the list. The root node is rendered but skipped in display (offset by 1).

## Dependencies

| Package | Version | Used for |
|---------|---------|----------|
| `bubbletea` | v1.3.10 | TUI framework |
| `lipgloss` | v1.1.0 | Styling, layout |
| `chroma/v2` | v2.27.0 | Syntax highlighting |
| `bubbles/viewport` | v1.0.0 | Scrollable terminal output |
| Go toolchain | 1.26.5 | |

## Extending

- **New panel**: Add field + handle*Key + add to layout in View() + add to cycleFocus
- **New language**: Add to `ServerForLang()`, `LangFromPath()`, and chroma lexer map in `highlightLine()`
- **New LSP feature**: Add request method + handler + UI panel
- **File tree features**: Add file filtering, rename/delete operations, git status indicators
