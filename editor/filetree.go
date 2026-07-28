package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// File tree node
// ═══════════════════════════════════════════════════════════════════════════════

// FileNode represents a single entry in the file tree.
type FileNode struct {
	name     string
	path     string
	isDir    bool
	expanded bool
	children []*FileNode
	depth    int
}

// FileTree is a navigable file explorer tree.
type FileTree struct {
	root      *FileNode
	cursor    int // index into visible slice
	visible   []*FileNode
	width     int
	height    int
	showHidden bool
	filterExt string // if set, only show files with this extension
}

// NewFileTree creates a file tree rooted at the given directory.
func NewFileTree(rootPath string) *FileTree {
	ft := &FileTree{
		width:  30,
		height: 20,
	}
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		abs = rootPath
	}
	ft.root = &FileNode{
		name:     filepath.Base(abs),
		path:     abs,
		isDir:    true,
		expanded: true,
		depth:    0,
	}
	ft.loadChildren(ft.root)
	ft.flatten()
	return ft
}

// ── Tree building ─────────────────────────────────────────────────────────

func (ft *FileTree) loadChildren(parent *FileNode) {
	entries, err := os.ReadDir(parent.path)
	if err != nil {
		parent.children = nil
		return
	}

	// Sort: directories first, then by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir() // dirs first
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	parent.children = make([]*FileNode, 0, len(entries))
	for _, e := range entries {
		// Skip hidden files unless showHidden is true
		if !ft.showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		node := &FileNode{
			name:     e.Name(),
			path:     filepath.Join(parent.path, e.Name()),
			isDir:    e.IsDir(),
			expanded: false,
			depth:    parent.depth + 1,
		}
		parent.children = append(parent.children, node)
	}
}

func (ft *FileTree) flatten() {
	ft.visible = nil
	ft.flattenNode(ft.root)
	// Clamp cursor
	if ft.cursor >= len(ft.visible) {
		ft.cursor = len(ft.visible) - 1
	}
	if ft.cursor < 0 {
		ft.cursor = 0
	}
}

func (ft *FileTree) flattenNode(node *FileNode) {
	ft.visible = append(ft.visible, node)
	if node.isDir && node.expanded {
		for _, child := range node.children {
			ft.flattenNode(child)
		}
	}
}

// ── Navigation ─────────────────────────────────────────────────────────────

func (ft *FileTree) MoveUp() {
	if ft.cursor > 0 {
		ft.cursor--
	}
}

func (ft *FileTree) MoveDown() {
	if ft.cursor < len(ft.visible)-1 {
		ft.cursor++
	}
}

// Toggle expands/collapses the current node if it's a directory.
func (ft *FileTree) Toggle() {
	node := ft.Current()
	if node == nil || !node.isDir {
		return
	}
	node.expanded = !node.expanded
	if node.expanded {
		ft.loadChildren(node)
	}
	ft.flatten()
}

// Open returns the path of the current node. Returns empty string if the node
// is a directory (call Toggle instead). Returns the full file path.
func (ft *FileTree) Open() string {
	node := ft.Current()
	if node == nil {
		return ""
	}
	if node.isDir {
		ft.Toggle()
		return ""
	}
	return node.path
}

// SetRoot changes the root directory and rebuilds the tree.
func (ft *FileTree) SetRoot(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	ft.root = &FileNode{
		name:     filepath.Base(abs),
		path:     abs,
		isDir:    true,
		expanded: true,
		depth:    0,
	}
	ft.loadChildren(ft.root)
	ft.cursor = 0
	ft.flatten()
}

func (ft *FileTree) Current() *FileNode {
	if ft.cursor < 0 || ft.cursor >= len(ft.visible) {
		return nil
	}
	return ft.visible[ft.cursor]
}

func (ft *FileTree) CurrentPath() string {
	n := ft.Current()
	if n == nil {
		return ""
	}
	return n.path
}

// ── View ───────────────────────────────────────────────────────────────────

var (
	expBg     = lipgloss.Color("#1E1E2E")
	expSurface = lipgloss.Color("#2A2A3E")
	expAccent  = lipgloss.Color("#FF75B7")
	expBlue    = lipgloss.Color("#7C5CFC")
	expMuted   = lipgloss.Color("#6C6C80")
	expText    = lipgloss.Color("#CDD6F4")

	explorerTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(expAccent).
			Background(expSurface).
			Padding(0, 2).
			Width(30)

	fileItem = lipgloss.NewStyle().
			Foreground(expText).
			PaddingLeft(1)

	dirItem = lipgloss.NewStyle().
			Foreground(expBlue).
			PaddingLeft(1)

	dirOpenItem = lipgloss.NewStyle().
			Foreground(expAccent).
			Bold(true).
			PaddingLeft(1)

	cursorBg = lipgloss.NewStyle().
			Background(lipgloss.Color("#2A2A3E")).
			Foreground(expAccent).
			Bold(true).
			PaddingLeft(1)

	cursorFileBg = lipgloss.NewStyle().
			Background(lipgloss.Color("#2A2A3E")).
			Foreground(expText).
			PaddingLeft(1)

	connectorStyle = lipgloss.NewStyle().
			Foreground(expMuted)
)

func (ft *FileTree) View() string {
	if ft.root == nil {
		return ""
	}

	var sb strings.Builder
	title := filepath.Base(ft.root.path)
	sb.WriteString(explorerTitle.Render("📁 " + title))
	sb.WriteString("\n")

	start := 0
	if ft.cursor > ft.height-2 {
		start = ft.cursor - ft.height + 2
	}
	end := start + ft.height - 2
	if end > len(ft.visible) {
		end = len(ft.visible)
	}

	for i := start; i < end; i++ {
		node := ft.visible[i]
		if i == 0 {
			continue // skip root node
		}

		// Build tree connectors
		var indent strings.Builder
		depth := node.depth - 1

		for d := 0; d < depth; d++ {
			// Check if this depth level has a sibling after this node
			isLast := true
			for j := i + 1; j < len(ft.visible); j++ {
				if ft.visible[j].depth-1 == d {
					isLast = false
					break
				}
				if ft.visible[j].depth-1 < d {
					break
				}
			}
			if isLast {
				indent.WriteString(connectorStyle.Render("  "))
			} else {
				indent.WriteString(connectorStyle.Render("│ "))
			}
		}

		// Check if this is the last child
		isLastChild := true
		for j := i + 1; j < len(ft.visible); j++ {
			if ft.visible[j].depth-1 == depth {
				isLastChild = false
				break
			}
			if ft.visible[j].depth-1 < depth {
				break
			}
		}

		if isLastChild {
			indent.WriteString(connectorStyle.Render("└──"))
		} else {
			indent.WriteString(connectorStyle.Render("├──"))
		}

		// Icon
		icon := " "
		if node.isDir {
			if node.expanded {
				icon = "📂"
			} else {
				icon = "📁"
			}
		} else {
			icon = " "
		}

		line := indent.String() + icon + " " + node.name

		var style lipgloss.Style
		if i == ft.cursor {
			if node.isDir {
				style = cursorBg
			} else {
				style = cursorFileBg
			}
		} else if node.isDir {
			if node.expanded {
				style = dirOpenItem
			} else {
				style = dirItem
			}
		} else {
			style = fileItem
		}

		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	return sb.String()
}

// ── File tree scrolling / resize ──────────────────────────────────────────

func (ft *FileTree) Resize(w, h int) {
	ft.width = w
	ft.height = h
}
