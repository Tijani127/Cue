package editor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Messages
// ═══════════════════════════════════════════════════════════════════════════════

type termOutputMsg struct {
	line string
}

type termDoneMsg struct {
	err error
}

// ═══════════════════════════════════════════════════════════════════════════════
// Terminal runner
// ═══════════════════════════════════════════════════════════════════════════════

// Terminal is a command runner panel. Each command runs in a fresh shell at the
// terminal's tracked cwd. cd commands are intercepted so subsequent commands
// run in the right directory. Cross-platform.
type Terminal struct {
	history    []string
	input      string
	cursor     int
	historyIdx int
	cmdHistory []string
	running    bool
	cmd        *exec.Cmd
	outputCh   chan termOutputMsg
	doneCh     chan error
	width      int
	height     int
	vp         viewport.Model
	cwd        string
	shell      string
	prompt     string
}

func NewTerminal() *Terminal {
	vp := viewport.New(80, 10)
	vp.Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#CDD6F4")).
		Background(lipgloss.Color("#1E1E2E")).
		PaddingLeft(1)

	cwd, _ := os.Getwd()
	shell := detectShell()
	return &Terminal{
		history:    []string{},
		input:      "",
		cmdHistory: []string{},
		historyIdx: -1,
		outputCh:   make(chan termOutputMsg, 256),
		doneCh:     make(chan error, 1),
		vp:         vp,
		cwd:        cwd,
		shell:      shell,
		prompt:     formatPrompt(cwd, shell),
	}
}

// detectShell checks for available shells in priority order:
// nu > pwsh > bash > zsh > cmd (cross-platform).
func detectShell() string {
	preferred := []string{"nu", "pwsh", "bash", "zsh", "cmd"}
	for _, name := range preferred {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
		// On Windows, try with .exe
		if runtime.GOOS == "windows" {
			if _, err := exec.LookPath(name + ".exe"); err == nil {
				return name
			}
		}
	}
	return "cmd"
}

func formatPrompt(cwd, shell string) string {
	home, _ := os.UserHomeDir()
	display := cwd
	if home != "" && strings.HasPrefix(display, home) {
		display = "~" + strings.TrimPrefix(display, home)
	}
	switch shell {
	case "pwsh", "powershell":
		return fmt.Sprintf("%s ❯ ", display)
	case "cmd":
		return fmt.Sprintf("%s>", display)
	default:
		return fmt.Sprintf("%s ❯ ", display)
	}
}

func shellCommand(shell, cmd string) (string, []string) {
	switch shell {
	case "nu":
		return "nu", []string{"-c", cmd}
	case "pwsh":
		return "pwsh", []string{"-NoLogo", "-NoProfile", "-Command", cmd}
	case "powershell":
		return "powershell", []string{"-NoLogo", "-NoProfile", "-Command", cmd}
	case "cmd":
		return "cmd", []string{"/c", cmd}
	case "zsh":
		return "zsh", []string{"-c", cmd}
	default:
		return "bash", []string{"-c", cmd}
	}
}

// ── cd tracking ────────────────────────────────────────────────────────────

// parseCd handles cd/pushd commands so cwd stays in sync. Returns true if
// the command was a directory change.
func (t *Terminal) parseCd(cmdStr string) bool {
	fields := strings.Fields(strings.TrimSpace(cmdStr))
	if len(fields) == 0 {
		return false
	}
	op := fields[0]
	if op != "cd" && op != "pushd" && op != "popd" {
		return false
	}
	if op == "popd" {
		// Can't track pushd/popd stack without a shell — just keep current cwd
		return true
	}
	// cd or pushd
	path := "~"
	if len(fields) > 1 {
		path = fields[1]
	}
	// Expand ~
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home + path[1:]
		}
	}
	// Handle relative
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.cwd, path)
	}
	// Clean
	path = filepath.Clean(path)

	// Verify it exists and is a directory
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		t.cwd = path
		t.prompt = formatPrompt(t.cwd, t.shell)
		t.writeCwdFile()
	} else {
		t.addOutput(fmt.Sprintf("cd: %s: no such directory", path))
	}
	return true
}

// writeCwdFile writes the current cwd to CUE_CWD_FILE so the file explorer
// can follow along.
func (t *Terminal) writeCwdFile() {
	path := os.Getenv("CUE_CWD_FILE")
	if path == "" {
		return
	}
	os.WriteFile(path, []byte(t.cwd), 0644)
}

// ── Execution ──────────────────────────────────────────────────────────────

func (t *Terminal) startCmd(cmdStr string) {
	// Intercept cd
	if t.parseCd(cmdStr) {
		return
	}
	if t.running {
		t.addOutput("error: command already running")
		return
	}

	name, args := shellCommand(t.shell, cmdStr)
	t.cmd = exec.Command(name, args...)
	t.cmd.Dir = t.cwd

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		t.addOutput("error: " + err.Error())
		return
	}
	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		t.addOutput("error: " + err.Error())
		return
	}

	if err := t.cmd.Start(); err != nil {
		t.addOutput("error: " + err.Error())
		return
	}

	t.running = true
	t.cmdHistory = append(t.cmdHistory, cmdStr)
	t.historyIdx = -1

	t.addOutput(t.prompt + cmdStr)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.outputCh <- termOutputMsg{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			t.outputCh <- termOutputMsg{line: "[read error: " + err.Error() + "]"}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.outputCh <- termOutputMsg{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			t.outputCh <- termOutputMsg{line: "[read error: " + err.Error() + "]"}
		}
	}()

	go func() {
		err := t.cmd.Wait()
		t.running = false
		t.doneCh <- err
	}()
}

func (t *Terminal) addOutput(line string) {
	t.history = append(t.history, line)
	if len(t.history) > 5000 {
		t.history = t.history[len(t.history)-5000:]
	}
	t.vp.SetContent(strings.Join(t.history, "\n"))
	t.vp.GotoBottom()
}

func (t *Terminal) HandleOutput(line string) { t.addOutput(line) }

func (t *Terminal) HandleDone(err error) {
	t.running = false
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.addOutput(fmt.Sprintf("exit: %d", exitErr.ExitCode()))
		} else {
			t.addOutput("error: " + err.Error())
		}
	}
	t.cmd = nil
}

// ── Input ──────────────────────────────────────────────────────────────────

func (t *Terminal) InsertRune(r rune) {
	if t.running {
		return
	}
	t.input = t.input[:t.cursor] + string(r) + t.input[t.cursor:]
	t.cursor++
}

func (t *Terminal) DeleteBackward() {
	if t.cursor > 0 && !t.running {
		t.input = t.input[:t.cursor-1] + t.input[t.cursor:]
		t.cursor--
	}
}

func (t *Terminal) DeleteForward() {
	if t.cursor < len(t.input) && !t.running {
		t.input = t.input[:t.cursor] + t.input[t.cursor+1:]
	}
}

func (t *Terminal) MoveLeft()  { if t.cursor > 0 { t.cursor-- } }
func (t *Terminal) MoveRight() { if t.cursor < len(t.input) { t.cursor++ } }
func (t *Terminal) MoveHome()  { t.cursor = 0 }
func (t *Terminal) MoveEnd()   { t.cursor = len(t.input) }

func (t *Terminal) HistoryPrev() {
	if len(t.cmdHistory) == 0 {
		return
	}
	if t.historyIdx == -1 {
		t.historyIdx = len(t.cmdHistory) - 1
	} else if t.historyIdx > 0 {
		t.historyIdx--
	}
	t.input = t.cmdHistory[t.historyIdx]
	t.cursor = len(t.input)
}

func (t *Terminal) HistoryNext() {
	if t.historyIdx == -1 {
		return
	}
	t.historyIdx++
	if t.historyIdx >= len(t.cmdHistory) {
		t.historyIdx = len(t.cmdHistory) - 1
		t.input = ""
		t.cursor = 0
		return
	}
	t.input = t.cmdHistory[t.historyIdx]
	t.cursor = len(t.input)
}

func (t *Terminal) Execute() {
	cmd := strings.TrimSpace(t.input)
	t.input = ""
	t.cursor = 0
	if cmd == "" {
		return
	}
	t.startCmd(cmd)
}

// ── View ───────────────────────────────────────────────────────────────────

var (
	termTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#4AFA9A")).
			Background(lipgloss.Color("#1E1E2E")).
			Padding(0, 2)

	termInputPrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4AFA9A")).
			Bold(true).
			PaddingLeft(1)

	termInputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CDD6F4"))

	termCursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FF75B7"))

	runningIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFB347")).
				Bold(true)
)

func (t *Terminal) View() string {
	if t.width == 0 {
		t.width = 80
	}
	if t.height == 0 {
		t.height = 10
	}

	var sb strings.Builder

	// Title bar
	if t.running {
		sb.WriteString(termTitleStyle.Render("● terminal"))
	} else {
		sb.WriteString(termTitleStyle.Render("⊢ terminal"))
	}
	sb.WriteString("\n")

	// Output viewport
	t.vp.Width = t.width
	vpHeight := t.height - 3
	if vpHeight < 1 {
		vpHeight = 1
	}
	t.vp.Height = vpHeight
	sb.WriteString(t.vp.View())
	sb.WriteString("\n")

	// Input line
	if !t.running {
		sb.WriteString(termInputPrompt.Render(t.prompt))
		display := t.input
		if t.cursor >= 0 && t.cursor <= len(display) {
			before := display[:t.cursor]
			after := display[t.cursor:]
			sb.WriteString(termInputStyle.Render(before))
			if t.cursor < len(display) {
				charAt := display[t.cursor : t.cursor+1]
				sb.WriteString(
					lipgloss.NewStyle().
						Background(lipgloss.Color("#FF75B7")).
						Foreground(lipgloss.Color("#000")).
						Render(charAt),
				)
			} else {
				sb.WriteString(termCursorStyle.Render(" "))
			}
			sb.WriteString(termInputStyle.Render(after))
		} else {
			sb.WriteString(termInputStyle.Render(display))
		}
	} else {
		sb.WriteString(fmt.Sprintf("  ◌ running…"))
	}

	return sb.String()
}

func (t *Terminal) Resize(w, h int) {
	t.width = w
	t.height = h
}
