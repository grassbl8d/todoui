package todoui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Idea is a quick thought captured locally (not synced to Todoist, for now).
// Each idea doubles as the root of a keyboard-driven mind map: Children holds
// the branches hanging off the idea text.
type Idea struct {
	Text        string      `json:"text"`
	At          string      `json:"at"`                   // RFC3339 capture time
	Children    []*MindNode `json:"children,omitempty"`   // mind-map branches off this idea
	Color       int         `json:"color,omitempty"`      // outline palette index (0 = default)
	BG          int         `json:"bg,omitempty"`         // background palette index (0 = none)
	ProjectID   string      `json:"project_id,omitempty"` // the one project this idea's tasks go to
	ProjectName string      `json:"project_name,omitempty"`
}

// MindNode is one node in an idea's mind map. The tree is unbounded in depth;
// Collapsed hides a node's children in the editor without deleting them.
type MindNode struct {
	Text      string      `json:"text"`
	Children  []*MindNode `json:"children,omitempty"`
	Collapsed bool        `json:"collapsed,omitempty"`
	IsTask    bool        `json:"task,omitempty"`    // flagged as an actionable task (t)
	TaskID    string      `json:"task_id,omitempty"` // linked Todoist task (set on convert/T)
	Done      bool        `json:"done,omitempty"`    // task completed (locally or via sync)
	Color     int         `json:"color,omitempty"`   // outline palette index (0 = default)
	BG        int         `json:"bg,omitempty"`      // background palette index (0 = none)
}

// mindColorCount is how many colours the c/C/b/B cycle steps through (plus the
// implicit 0 = default), defined alongside the palette in mindmap_view.go.
const mindColorCount = 10

// nextMindColor advances a palette index, wrapping past the last colour back to
// 0 (the default / no-colour state).
func nextMindColor(i int) int { return (i + 1) % (mindColorCount + 1) }

// setSubtreeOutline sets the outline colour index on every node in the subtrees.
func setSubtreeOutline(children []*MindNode, idx int) {
	for _, n := range children {
		n.Color = idx
		setSubtreeOutline(n.Children, idx)
	}
}

// setSubtreeBG sets the background colour index on every node in the subtrees.
func setSubtreeBG(children []*MindNode, idx int) {
	for _, n := range children {
		n.BG = idx
		setSubtreeBG(n.Children, idx)
	}
}

// unlinkMindTasks clears the Todoist link (and done state) on every node in the
// subtrees, leaving the task flags intact.
func unlinkMindTasks(children []*MindNode) {
	for _, n := range children {
		n.TaskID = ""
		n.Done = false
		unlinkMindTasks(n.Children)
	}
}

// collectMindTasks returns task-flagged nodes that still need a Todoist task
// created (depth-first order); already-linked or empty nodes are skipped so
// re-running the convert never duplicates tasks.
func (i Idea) collectMindTasks() []*MindNode {
	var out []*MindNode
	var walk func(ns []*MindNode)
	walk = func(ns []*MindNode) {
		for _, n := range ns {
			if n.IsTask && n.Text != "" && n.TaskID == "" {
				out = append(out, n)
			}
			walk(n.Children)
		}
	}
	walk(i.Children)
	return out
}

// countNodes returns the total number of descendant nodes under an idea.
func (i Idea) countNodes() int {
	var walk func(ns []*MindNode) int
	walk = func(ns []*MindNode) int {
		n := len(ns)
		for _, c := range ns {
			n += walk(c.Children)
		}
		return n
	}
	return walk(i.Children)
}

// insertMindNode returns children with n inserted at idx (clamped).
func insertMindNode(children []*MindNode, idx int, n *MindNode) []*MindNode {
	if idx < 0 {
		idx = 0
	}
	if idx > len(children) {
		idx = len(children)
	}
	children = append(children, nil)
	copy(children[idx+1:], children[idx:])
	children[idx] = n
	return children
}

// removeMindNode deletes target from the subtree rooted at *children, searching
// recursively. It reports whether the node was found and removed.
func removeMindNode(children *[]*MindNode, target *MindNode) bool {
	s := *children
	for i, n := range s {
		if n == target {
			*children = append(s[:i:i], s[i+1:]...)
			return true
		}
		if removeMindNode(&n.Children, target) {
			return true
		}
	}
	return false
}

func ideasPath() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "ideas.json")
}

// LoadIdeas reads captured ideas, newest first.
func LoadIdeas() []Idea {
	b, err := os.ReadFile(ideasPath())
	if err != nil {
		return nil
	}
	var ideas []Idea
	if json.Unmarshal(b, &ideas) != nil {
		return nil
	}
	return ideas
}

// SaveIdeas persists the ideas list.
func SaveIdeas(ideas []Idea) {
	if p := ideasPath(); p != "" {
		if b, err := json.Marshal(ideas); err == nil {
			_ = os.WriteFile(p, b, 0o600)
		}
	}
}

// addIdea returns the list with a new idea prepended (newest first).
func addIdea(ideas []Idea, text string) []Idea {
	return append([]Idea{{Text: text, At: nowStamp()}}, ideas...)
}
