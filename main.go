package main

import (
	"fmt"
	"os"

	"cue/editor"

	tea "github.com/charmbracelet/bubbletea"
)

// cueVersion is the app version. Override at build time with:
//
//	go build -ldflags "-X main.cueVersion=v1.2.3"
var cueVersion = "0.2.0"

func main() {
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			fmt.Println("Cue " + cueVersion)
			return
		}
		if a == "--help" || a == "-h" {
			printHelp()
			return
		}
	}

	files := os.Args[1:]
	ed := editor.NewEditor(files)

	p := tea.NewProgram(ed, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Codex error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Cue " + cueVersion + " - terminal code editor")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cue <file1> [file2 ...]   Open file(s)")
	fmt.Println("  cue                      Start with an untitled buffer")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help      Show this help")
	fmt.Println("  -v, --version   Print version")
}
