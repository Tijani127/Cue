package main

import (
	"fmt"
	"os"

	"cue/editor"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	files := os.Args[1:]
	ed := editor.NewEditor(files)

	p := tea.NewProgram(ed, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Codex error: %v\n", err)
		os.Exit(1)
	}
}
