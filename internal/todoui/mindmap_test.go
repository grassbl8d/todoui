package todoui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// key presses a single rune; keyType presses a named key (tab, enter, esc…).
func key(m model, r rune) model {
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return nm.(model)
}
func keyType(m model, t tea.KeyType) model {
	nm, _ := m.Update(tea.KeyMsg{Type: t})
	return nm.(model)
}
func typeText(m model, s string) model {
	for _, r := range s {
		m = key(m, r)
	}
	return m
}

func mindModel(t *testing.T) model {
	t.Setenv("HOME", t.TempDir()) // isolate SaveIdeas writes
	m := newTestModel()
	m.width, m.height = 100, 40
	m.ideas = []Idea{{Text: "Buy a house", At: nowStamp()}}
	m.ideaCursor = 0
	m.mode = modeIdeaList
	return m
}

func TestMindMapZoomOverlayShowsFullText(t *testing.T) {
	m := mindModel(t)
	full := "is it actually possible for claude to add screen shots"
	m.ideas = []Idea{{Text: "Root idea", At: nowStamp(), Children: []*MindNode{
		{Text: full},
		{Text: "second node"},
	}}}
	m.ideaCursor = 0
	m.width = 120
	m = keyType(m, tea.KeyEnter) // open the map (cursor on root idea)
	m = key(m, 'l')              // descend onto the long child node
	if m.mindRows()[m.mindCursor].node.Text != full {
		t.Fatal("expected cursor on the long child node")
	}
	// Before zooming, the long label is truncated inside its box.
	if strings.Contains(m.View(), full) {
		t.Fatal("map should truncate the long label before zooming")
	}

	// z overlays the selected node's full text on top of the map.
	m = key(m, 'z')
	if !m.mindZoom {
		t.Fatal("z should turn on the zoom overlay")
	}
	if m.mode != modeMindMap {
		t.Fatalf("zoom stays in the map mode, got %d", m.mode)
	}
	v := m.View()
	if !strings.Contains(v, full) {
		t.Fatalf("zoom overlay should show the full node text, got:\n%s", v)
	}
	// The map renders behind the overlay: the title and an uncovered sibling box
	// are still visible.
	if !strings.Contains(v, "Mind map") || !strings.Contains(v, "second node") {
		t.Fatalf("map should remain visible behind the overlay, got:\n%s", v)
	}

	// z again closes the overlay; the truncated label is back.
	m = key(m, 'z')
	if m.mindZoom {
		t.Fatal("z again should close the overlay")
	}
	if strings.Contains(m.View(), full) {
		t.Fatal("closing the overlay should return to the truncated map")
	}
}

func TestIdeaListCaptureAndLegend(t *testing.T) {
	m := mindModel(t)
	m.ideas = nil // empty ideas list
	m.mode = modeIdeaList

	// The empty state must still advertise the shortcuts.
	v := m.View()
	if !strings.Contains(v, "i catch") || !strings.Contains(v, "b back") || !strings.Contains(v, ". home") {
		t.Fatalf("empty ideas list should show i/b/h shortcuts, got:\n%s", v)
	}

	// i captures a new idea (opens the capture input).
	m = key(m, 'i')
	if m.mode != modeIdeaAdd {
		t.Fatalf("i should open idea capture, mode=%d", m.mode)
	}
}

func TestIdeaCaptureLandsOnIdeaList(t *testing.T) {
	m := mindModel(t)
	m.ideas = nil
	m.mode = modeIdeaList
	m = key(m, 'i') // open capture
	if m.mode != modeIdeaAdd {
		t.Fatalf("i should open capture, mode=%d", m.mode)
	}
	m = typeText(m, "ship it")
	m = keyType(m, tea.KeyEnter) // save
	if m.mode != modeIdeaList {
		t.Fatalf("after saving an idea, should land on the ideas list, mode=%d", m.mode)
	}
	if len(m.ideas) != 1 || m.ideas[0].Text != "ship it" {
		t.Fatalf("idea not saved: %+v", m.ideas)
	}
	if m.ideaCursor != 0 {
		t.Fatalf("cursor should highlight the new (newest) idea, got %d", m.ideaCursor)
	}
}

func TestIdeaListHomeKey(t *testing.T) {
	m := mindModel(t)
	m.mode = modeIdeaList
	m = key(m, '.') // home → back to the task list
	if m.mode != modeList {
		t.Fatalf(". should return to the task list, mode=%d", m.mode)
	}
}

func TestMindMapIGoesToIdeaList(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // open the map
	if m.mode != modeMindMap {
		t.Fatalf("expected map mode, got %d", m.mode)
	}
	m = key(m, 'I') // I → ideas list
	if m.mode != modeIdeaList {
		t.Fatalf("I in the map should go to the ideas list, mode=%d", m.mode)
	}
}

func TestMindEditDoesNotHijackI(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // map
	m = keyType(m, tea.KeyTab)   // add a child → editing a node
	if m.mode != modeMindEdit {
		t.Fatalf("tab should start editing, mode=%d", m.mode)
	}
	m = key(m, 'I') // typed into the node, must NOT jump to ideas
	if m.mode != modeMindEdit {
		t.Fatalf("I while editing a node should stay in edit, mode=%d", m.mode)
	}
}

func TestMindMapZoomToggleHelpEsc(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // open the map, cursor on the root idea

	m = key(m, 'z')
	if !m.mindZoom {
		t.Fatal("z should open the zoom overlay")
	}
	// H still opens help while zoomed, without losing the overlay.
	m = key(m, 'H')
	if m.mode != modeMindHelp {
		t.Fatalf("H should open help while zoomed, mode=%d", m.mode)
	}
	if !m.mindZoom {
		t.Fatal("opening help should not clear the zoom")
	}
	m = key(m, 'x') // any key closes the help → back to the zoomed map
	if m.mode != modeMindMap || !m.mindZoom {
		t.Fatalf("closing help should return to the zoomed map, mode=%d zoom=%v", m.mode, m.mindZoom)
	}
	// First esc closes the overlay (stays in the map); a second esc leaves.
	m = keyType(m, tea.KeyEsc)
	if m.mindZoom || m.mode != modeMindMap {
		t.Fatalf("esc should close the overlay but stay in the map, zoom=%v mode=%d", m.mindZoom, m.mode)
	}
	m = keyType(m, tea.KeyEsc)
	if m.mode != modeIdeaList {
		t.Fatalf("a second esc should return to the idea list, mode=%d", m.mode)
	}
}

func TestMindMapEnterOpensMap(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // open the selected idea as a mind map
	if m.mode != modeMindMap {
		t.Fatalf("enter on an idea should open the mind map, mode=%d", m.mode)
	}
	if rows := m.mindRows(); len(rows) != 1 || !rows[0].isRoot {
		t.Fatalf("a fresh map should have only the root row, got %d rows", len(rows))
	}
	if v := m.View(); !strings.Contains(v, "Mind map") || !strings.Contains(v, "Buy a house") {
		t.Fatalf("map view should show the title and root idea, got:\n%s", v)
	}
}

func TestMindMapTabAddsChildEnterAddsSibling(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // into the map (cursor on root)

	// Tab (in the map) starts editing a new child of the root.
	m = keyType(m, tea.KeyTab)
	if m.mode != modeMindEdit {
		t.Fatal("tab should start editing the new child node")
	}
	m = typeText(m, "Location")

	// Enter commits "Location" and chains into a new sibling (still editing).
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeMindEdit {
		t.Fatalf("enter should chain into a new sibling editor, mode=%d", m.mode)
	}
	m = typeText(m, "Budget")

	// Tab commits "Budget" and chains into a child of Budget.
	m = keyType(m, tea.KeyTab)
	if m.mode != modeMindEdit {
		t.Fatalf("tab should chain into a child editor, mode=%d", m.mode)
	}
	m = typeText(m, "Under 500k")

	// Enter commits the grandchild + opens an empty sibling; Esc drops it.
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyEsc)

	kids := m.ideas[0].Children
	if len(kids) != 2 || kids[0].Text != "Location" || kids[1].Text != "Budget" {
		t.Fatalf("expected root children [Location Budget], got %+v", kids)
	}
	if gk := kids[1].Children; len(gk) != 1 || gk[0].Text != "Under 500k" {
		t.Fatalf("tab should nest 'Under 500k' under Budget, got %+v", gk)
	}
}

func TestMindMapShiftReorderSiblings(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A"}, {Text: "B"}, {Text: "C"}}
	m = keyType(m, tea.KeyEnter) // open map

	// Move the cursor onto C (the last sibling).
	m = key(m, 'l') // descend to A
	for m.mindRows()[m.mindCursor].node.Text != "C" {
		m = key(m, 'j')
	}
	kids := m.ideas[0].Children
	if len(kids) != 3 || kids[0].Text != "A" || kids[2].Text != "C" {
		t.Fatalf("setup expected [A B C], got %+v", kids)
	}

	// Shift+Up swaps C with B → [A C B].
	m = keyType(m, tea.KeyShiftUp)
	kids = m.ideas[0].Children
	if kids[1].Text != "C" || kids[2].Text != "B" {
		t.Fatalf("shift+up should swap C above B, got %+v", kids)
	}
	// Cursor should follow the moved node (still C).
	if rows := m.mindRows(); rows[m.mindCursor].node.Text != "C" {
		t.Fatalf("cursor should follow the moved node C, on %q", rows[m.mindCursor].node.Text)
	}

	// Shift+Down puts C back below B → [A B C].
	m = keyType(m, tea.KeyShiftDown)
	kids = m.ideas[0].Children
	if kids[1].Text != "B" || kids[2].Text != "C" {
		t.Fatalf("shift+down should swap C below B, got %+v", kids)
	}
}

func TestMindMapShiftLeftPromotes(t *testing.T) {
	m := mindModel(t)
	// Root → A → G (grandchild under A).
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{{Text: "G"}}}}
	m = keyType(m, tea.KeyEnter) // open map

	// Move the cursor onto G (two columns in).
	m = key(m, 'l') // A
	m = key(m, 'l') // G
	if m.mindRows()[m.mindCursor].node.Text != "G" {
		t.Fatalf("expected cursor on G, got %q", m.mindRows()[m.mindCursor].node.Text)
	}

	// Shift+Left promotes G to be a sibling of A (top level).
	m = keyType(m, tea.KeyShiftLeft)
	kids := m.ideas[0].Children
	if len(kids) != 2 || kids[0].Text != "A" || kids[1].Text != "G" {
		t.Fatalf("shift+left should promote G next to A, got %+v", kids)
	}
	if len(kids[0].Children) != 0 {
		t.Fatalf("G should no longer be a child of A, got %+v", kids[0].Children)
	}

	// Promoting again: G is now top-level, so it can't go higher.
	m = keyType(m, tea.KeyShiftLeft)
	if !strings.Contains(m.status, "top level") {
		t.Fatalf("promoting a top-level node should report it can't go further, got %q", m.status)
	}
}

func TestGlobalProjectsHotkey(t *testing.T) {
	// p jumps to the projects list (view-by-project) from the mind map.
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // into the map
	m = key(m, 'p')
	if m.mode != modeProjectPick || m.pickIntent != pickView {
		t.Fatalf("p in the mind map should open the projects list (view), got mode=%v intent=%v", m.mode, m.pickIntent)
	}
}

func TestGlobalProjectsAndHomeFromMenus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)

	// p from the help screen → projects picker.
	m.mode = modeHelp
	m = key(m, 'p')
	if m.mode != modeProjectPick || m.pickIntent != pickView {
		t.Fatalf("p should open projects from help, got mode=%v", m.mode)
	}

	// . from the options screen → home (task list).
	m.mode = modeOptions
	m = key(m, '.')
	if m.mode != modeList {
		t.Fatalf(". should go home from options, got %v", m.mode)
	}
}

func TestMindMapHStaysParentNav(t *testing.T) {
	// In the mind map, h must remain parent navigation, NOT the global home jump.
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "child"}}
	m = keyType(m, tea.KeyEnter) // open map
	m = key(m, 'l')              // descend to the child
	m = key(m, 'h')              // back to the parent — should stay in the map
	if m.mode != modeMindMap {
		t.Fatalf("h in the mind map should stay in the map (parent nav), got %v", m.mode)
	}
}

func TestMindMapSearchAndCycle(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{
		{Text: "deploy alpha"}, {Text: "buy milk"}, {Text: "deploy beta"},
	}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter) // open map (cursor on root)

	// / opens search; typing + enter jumps to the first match.
	m = key(m, '/')
	if m.mode != modeMindSearch {
		t.Fatalf("/ should open search, got mode=%d", m.mode)
	}
	m = typeText(m, "deploy")
	m = keyType(m, tea.KeyEnter)
	if got := m.mindRows()[m.mindCursor].node.Text; got != "deploy alpha" {
		t.Fatalf("first match should be 'deploy alpha', got %q", got)
	}

	// n cycles to the next match, N back.
	m = key(m, 'n')
	if got := m.mindRows()[m.mindCursor].node.Text; got != "deploy beta" {
		t.Fatalf("n should jump to 'deploy beta', got %q", got)
	}
	m = key(m, 'n') // wraps back to the first
	if got := m.mindRows()[m.mindCursor].node.Text; got != "deploy alpha" {
		t.Fatalf("n should wrap to 'deploy alpha', got %q", got)
	}
	m = key(m, 'N') // previous wraps to the last
	if got := m.mindRows()[m.mindCursor].node.Text; got != "deploy beta" {
		t.Fatalf("N should wrap to 'deploy beta', got %q", got)
	}

	// esc in search clears the query without moving.
	m = key(m, '/')
	m = keyType(m, tea.KeyEsc)
	if m.mode != modeMindMap || m.mindSearch != "" {
		t.Fatalf("esc should cancel search and clear the query, mode=%d q=%q", m.mode, m.mindSearch)
	}
}

func TestMindMapCutCopyPaste(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{
		{Text: "A", Children: []*MindNode{{Text: "A-child"}}},
		{Text: "B"},
	}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter) // open map
	m = key(m, 'l')              // descend to A

	// Copy A (with its subtree), then paste as a child of A.
	m = key(m, 'c')
	if m.mindClip == nil || m.mindClip.Text != "A" {
		t.Fatalf("c should copy A to the clipboard, got %+v", m.mindClip)
	}
	m = key(m, 'v') // paste as child of A
	if got := m.ideas[0].Children[0].Children; len(got) != 2 || got[1].Text != "A" {
		t.Fatalf("v should paste a copy of A under A, got %+v", got)
	}
	// The pasted copy is deep — it brought A-child along.
	if pasted := m.ideas[0].Children[0].Children[1]; len(pasted.Children) != 1 || pasted.Children[0].Text != "A-child" {
		t.Fatalf("paste should deep-copy the subtree, got %+v", pasted)
	}

	// Cut B, then paste it as a sibling of A.
	m = key(m, 'r') // back to root
	m = key(m, 'l') // A
	// move down to B (last top-level child)
	for m.mindRows()[m.mindCursor].node == nil || m.mindRows()[m.mindCursor].node.Text != "B" {
		m = key(m, 'j')
	}
	m = key(m, 'x') // cut B
	if m.mindClip == nil || m.mindClip.Text != "B" {
		t.Fatalf("x should cut B to the clipboard, got %+v", m.mindClip)
	}
	found := false
	for _, c := range m.ideas[0].Children {
		if c.Text == "B" {
			found = true
		}
	}
	if found {
		t.Fatal("cut should remove B from the tree")
	}
	// Paste B as a sibling (V) somewhere — it should reappear in the tree.
	m = key(m, 'V')
	count := 0
	var walk func(ns []*MindNode)
	walk = func(ns []*MindNode) {
		for _, n := range ns {
			if n.Text == "B" {
				count++
			}
			walk(n.Children)
		}
	}
	walk(m.ideas[0].Children)
	if count != 1 {
		t.Fatalf("paste should re-add B exactly once, got %d", count)
	}
}

func TestMindMapStatusAutoClears(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: "A"}}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l') // select A
	m = key(m, 'c') // copy → sets a transient status
	m = key(m, 'v') // paste as child → "pasted as child"
	if m.status == "" {
		t.Fatal("paste should set a transient status")
	}
	seq := m.statusSeq

	// The matching auto-clear timer wipes it.
	nm, _ := m.Update(clearStatusMsg{seq: seq})
	m = nm.(model)
	if m.status != "" {
		t.Fatalf("clearStatusMsg with the current seq should clear the status, got %q", m.status)
	}

	// A stale timer (older seq) must NOT wipe a newer status.
	m = key(m, 'c') // new status, bumps the seq past `seq`
	newStatus := m.status
	nm, _ = m.Update(clearStatusMsg{seq: seq})
	m = nm.(model)
	if m.status != newStatus {
		t.Fatalf("a stale clear should leave a newer status, got %q want %q", m.status, newStatus)
	}
}

// TestMindMapBufferStickyStatus verifies the cut/copy "buffer is full" reminder
// stays up (no auto-clear timer) while the clipboard holds something, whereas the
// paste confirmation schedules its own auto-clear.
func TestMindMapBufferStickyStatus(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: "A"}}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l') // select A

	// Copy: the buffer reminder must be sticky — no auto-clear command scheduled.
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = res.(model)
	if m.status == "" {
		t.Fatal("copy should set the buffer reminder status")
	}
	if cmd != nil {
		t.Fatal("the buffer reminder should be sticky — no auto-clear command expected")
	}
	bufStatus := m.status

	// A stale timer (older seq) must not wipe the sticky reminder, and it has no
	// timer of its own, so it persists.
	nm, _ := m.Update(clearStatusMsg{seq: m.statusSeq - 1})
	m = nm.(model)
	if m.status != bufStatus {
		t.Fatalf("sticky buffer reminder should persist, got %q", m.status)
	}

	// Paste: a one-shot confirmation that DOES schedule its own auto-clear.
	res, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = res.(model)
	if m.status != "📋 pasted as child" {
		t.Fatalf("paste status = %q, want %q", m.status, "📋 pasted as child")
	}
	if cmd == nil {
		t.Fatal("paste confirmation should schedule an auto-clear command")
	}
	// The matching timer wipes the confirmation.
	nm, _ = m.Update(clearStatusMsg{seq: m.statusSeq})
	m = nm.(model)
	if m.status != "" {
		t.Fatalf("paste confirmation should auto-clear, got %q", m.status)
	}
}

func TestMindMapEscClearsSearch(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: "Flow"}, {Text: "x"}}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter)
	m = key(m, '/')
	m = typeText(m, "Flo")
	m = keyType(m, tea.KeyEnter)
	if m.mindSearch != "Flo" || m.status == "" {
		t.Fatalf("search should be active, q=%q status=%q", m.mindSearch, m.status)
	}

	// First esc clears the active search (query + status) and stays in the map.
	m = keyType(m, tea.KeyEsc)
	if m.mode != modeMindMap {
		t.Fatalf("esc with an active search should stay in the map, mode=%d", m.mode)
	}
	if m.mindSearch != "" || m.status != "" {
		t.Fatalf("esc should clear the search and its status, q=%q status=%q", m.mindSearch, m.status)
	}

	// A second esc leaves to the ideas list…
	m = keyType(m, tea.KeyEsc)
	if m.mode != modeIdeaList {
		t.Fatalf("second esc should leave to the ideas list, mode=%d", m.mode)
	}
	// …and re-entering the map carries no stale search status.
	m = keyType(m, tea.KeyEnter)
	if m.status != "" || m.mindSearch != "" {
		t.Fatalf("re-entering the map should be clean, q=%q status=%q", m.mindSearch, m.status)
	}
}

func TestIdeaListProjectsHotkey(t *testing.T) {
	m := mindModel(t) // starts in modeIdeaList
	m = key(m, 'p')
	if m.mode != modeProjectPick || m.pickIntent != pickView {
		t.Fatalf("p in the ideas list should open the projects list, got mode=%d", m.mode)
	}
}

func TestMindMapFullLabelToggle(t *testing.T) {
	m := mindModel(t)
	m.settings.NodeLabelLen = 26
	long := "Should be able to generate a full compose file from scratch"
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: long}}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l')

	// Normal view truncates the long label.
	if strings.Contains(m.View(), long) {
		t.Fatal("normal view should truncate the long label")
	}

	// d toggles full labels on → the whole label shows.
	m = key(m, 'd')
	if !m.mindFullLabels {
		t.Fatal("d should turn on full labels")
	}
	if !strings.Contains(m.View(), long) {
		t.Fatalf("full labels should show the whole text, got:\n%s", m.View())
	}

	// d again turns it back off (truncated again).
	m = key(m, 'd')
	if m.mindFullLabels || strings.Contains(m.View(), long) {
		t.Fatal("d again should restore truncation")
	}
}

func TestNodeLabelWidthMenuCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.settings.NodeLabelLen = 48
	m.mode = modeOptions
	m.optCursor = 9 // "Node label width"
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.settings.NodeLabelLen != 64 { // 48 → 64 in the cycle
		t.Fatalf("cycling from 48 should give 64, got %d", m.settings.NodeLabelLen)
	}
}

func TestMindMapCollapsedBadgeSurvivesLongLabel(t *testing.T) {
	m := mindModel(t)
	m.settings.NodeLabelLen = 26 // force truncation regardless of the default width
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{
		{Text: "Show the current Kube Context and the routes", Collapsed: true,
			Children: []*MindNode{{Text: "the only child"}}},
	}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l') // select the long collapsed node

	v := m.View()
	// The long label is truncated, but the single-child indicator must still show.
	if !strings.Contains(v, "…") {
		t.Fatal("expected the long label to be truncated")
	}
	if !strings.Contains(v, "(+1)") {
		t.Fatalf("the (+1) badge must survive truncation, got:\n%s", v)
	}
}

func TestMindMapOverviewFullTextAndSearch(t *testing.T) {
	m := mindModel(t)
	long := "Show the current Kube Context and the available projects too"
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: long}}}}
	m.ideaCursor = 0
	m.width, m.height = 120, 20
	m = keyType(m, tea.KeyEnter)

	// Normal view truncates the long label.
	if strings.Contains(m.View(), long) {
		t.Fatal("normal view should truncate the long label")
	}

	// Overview shows the full, untruncated text and hides the app header.
	m = key(m, 'Z')
	v := m.View()
	if !strings.Contains(v, long) {
		t.Fatalf("overview should show the full node text, got:\n%s", v)
	}
	if strings.Contains(v, "✓ Todoist") {
		t.Fatal("overview should be full screen (no app header)")
	}

	// Search works inside overview and keeps the overview on afterwards.
	m = key(m, '/')
	if m.mode != modeMindSearch {
		t.Fatalf("/ should open search in overview, mode=%d", m.mode)
	}
	m = typeText(m, "Kube")
	m = keyType(m, tea.KeyEnter)
	if !m.mindOverview {
		t.Fatal("search should keep the overview open")
	}
	if got := m.mindRows()[m.mindCursor].node.Text; got != long {
		t.Fatalf("search should jump to the matching node, got %q", got)
	}
}

func TestMindMapOverviewExpandsAndIsReadOnly(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{
		{Text: "Branch A", Collapsed: true, Children: []*MindNode{{Text: "Hidden G"}}},
		{Text: "Branch B"},
	}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter) // open map

	// Normal view: the collapsed branch hides its child.
	if strings.Contains(m.View(), "Hidden G") {
		t.Fatal("collapsed child should be hidden in the normal view")
	}

	// Z opens the read-only, all-expanded overview.
	m = key(m, 'Z')
	if !m.mindOverview {
		t.Fatal("Z should enable the overview")
	}
	v := m.View()
	if !strings.Contains(v, "Hidden G") {
		t.Fatalf("overview should expand collapsed branches, got:\n%s", v)
	}
	// Floating overview: all chrome is stripped — no app header, no indicator,
	// no footer shortcuts — just the map.
	if strings.Contains(v, "✓ Todoist") {
		t.Fatal("overview should hide the app header")
	}
	if strings.Contains(v, "OVERVIEW MODE") || strings.Contains(v, "Z/esc close") {
		t.Fatal("floating overview should drop the indicator and footer")
	}

	// Read-only: editing/structural keys are ignored (d must not delete).
	before := len(m.ideas[0].Children)
	m = key(m, 'd')
	if m.mode != modeMindMap || m.mindOverview != true {
		t.Fatal("d in overview should be ignored (no delete-confirm, stay in overview)")
	}
	if len(m.ideas[0].Children) != before {
		t.Fatal("d in overview must not change the tree")
	}

	// Z again closes the overview.
	m = key(m, 'Z')
	if m.mindOverview {
		t.Fatal("Z again should close the overview")
	}
}

func TestMindMapEnterChainsSiblings(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // open map (cursor on root)
	m = keyType(m, tea.KeyTab)   // new child, editing
	m = typeText(m, "first")

	// Enter commits "first" and opens an empty sibling editor (no Enter+Enter).
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeMindEdit {
		t.Fatalf("enter should chain into a new sibling editor, mode=%d", m.mode)
	}
	if kids := m.ideas[0].Children; len(kids) != 2 || kids[0].Text != "first" || kids[1].Text != "" {
		t.Fatalf("enter should commit 'first' and add an empty sibling, got %+v", kids)
	}
	m = typeText(m, "second")

	// Enter on the empty trailing node would chain again; instead Esc finishes.
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyEsc)

	kids := m.ideas[0].Children
	if len(kids) != 2 || kids[0].Text != "first" || kids[1].Text != "second" {
		t.Fatalf("enter-chaining should yield siblings [first second], got %+v", kids)
	}
}

func TestMindMapUndo(t *testing.T) {
	m := mindModel(t)
	m.ideas = []Idea{{Text: "Root", At: nowStamp(), Children: []*MindNode{{Text: "A"}, {Text: "B"}}}}
	m.ideaCursor = 0
	m = keyType(m, tea.KeyEnter) // open map
	m = key(m, 'l')              // select A

	// Copy A and paste it as a child → 3 nodes under A's parent path.
	m = key(m, 'c')
	m = key(m, 'v') // paste as child of A
	if len(m.ideas[0].Children[0].Children) != 1 {
		t.Fatalf("paste should add a child under A, got %+v", m.ideas[0].Children[0].Children)
	}

	// u undoes the paste.
	m = key(m, 'u')
	if len(m.ideas[0].Children[0].Children) != 0 {
		t.Fatalf("undo should remove the pasted child, got %+v", m.ideas[0].Children[0].Children)
	}
	if !strings.Contains(m.status, "undone") {
		t.Fatalf("undo should report 'undone', got %q", m.status)
	}

	// Delete B (immediate, no confirm), then undo restores it.
	for m.mindRows()[m.mindCursor].node == nil || m.mindRows()[m.mindCursor].node.Text != "B" {
		m = key(m, 'j')
	}
	m = keyType(m, tea.KeyBackspace)
	if len(m.ideas[0].Children) != 1 {
		t.Fatalf("delete should leave 1 child, got %+v", m.ideas[0].Children)
	}
	m = key(m, 'u')
	if len(m.ideas[0].Children) != 2 || m.ideas[0].Children[1].Text != "B" {
		t.Fatalf("undo should restore deleted B, got %+v", m.ideas[0].Children)
	}

	// Undo with an empty stack reports nothing to undo.
	for i := 0; i < 5; i++ {
		m = key(m, 'u')
	}
	if !strings.Contains(m.status, "nothing to undo") {
		t.Fatalf("exhausting the undo stack should say so, got %q", m.status)
	}
}

func TestMindMapEscDiscardsEmptyNewNode(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyTab) // new child, editing
	m = keyType(m, tea.KeyEsc) // cancel before typing
	if len(m.ideas[0].Children) != 0 {
		t.Fatalf("esc on a brand-new empty node should discard it, got %+v", m.ideas[0].Children)
	}
	if m.mode != modeMindMap {
		t.Fatal("esc should return to map navigation")
	}
}

func TestMindMapEscSavesTypedTextWithoutSibling(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyTab) // new child, editing
	m = typeText(m, "keep me")

	// Esc with text typed saves it and returns to the map — no extra sibling.
	m = keyType(m, tea.KeyEsc)
	if m.mode != modeMindMap {
		t.Fatalf("esc should return to map navigation, mode=%d", m.mode)
	}
	kids := m.ideas[0].Children
	if len(kids) != 1 || kids[0].Text != "keep me" {
		t.Fatalf("esc should save the typed node and add no sibling, got %+v", kids)
	}
}

func TestMindMapMarkAsTask(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "Call agent"}}
	m = keyType(m, tea.KeyEnter) // open map
	m = key(m, 'l')              // select the child

	m = key(m, 't') // mark as task
	if !m.ideas[0].Children[0].IsTask {
		t.Fatal("t should mark the selected node as a task")
	}
	if v := m.View(); !strings.Contains(v, "[ ]") {
		t.Fatalf("a task node should render a checkbox indicator, got:\n%s", v)
	}

	m = key(m, 't') // toggle back off
	if m.ideas[0].Children[0].IsTask {
		t.Fatal("t should unmark the task")
	}
}

func TestMindMapTWithNoTaskAlerts(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "An idea"}} // a plain node, not a task
	m = keyType(m, tea.KeyEnter)                         // open map

	m = key(m, 'T') // commit tasks — but nothing is marked
	if m.mode != modeMindMap {
		t.Fatal("T with no tasks should stay in the mind map, not open the project picker")
	}
	if !strings.Contains(m.status, "no tasks yet") {
		t.Fatalf("T should alert that no tasks are marked, got status %q", m.status)
	}
	if v := m.View(); !strings.Contains(v, "no tasks yet") {
		t.Fatalf("the alert should be visible in the mind-map view, got:\n%s", v)
	}
}

func TestMindMapDeleteImmediateAndUndo(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "Temp"}}
	m = keyType(m, tea.KeyEnter) // open map
	m = key(m, 'l')              // select Temp

	// Backspace deletes immediately (no confirmation) and offers undo.
	m = keyType(m, tea.KeyBackspace)
	if len(m.ideas[0].Children) != 0 {
		t.Fatalf("backspace should delete the node, got %+v", m.ideas[0].Children)
	}
	if m.mode != modeMindMap || !strings.Contains(m.status, "undo") {
		t.Fatalf("delete should stay in the map and mention undo, mode=%d status=%q", m.mode, m.status)
	}

	// u restores it.
	m = key(m, 'u')
	if len(m.ideas[0].Children) != 1 || m.ideas[0].Children[0].Text != "Temp" {
		t.Fatalf("undo should restore the deleted node, got %+v", m.ideas[0].Children)
	}

	// Delete (forward-delete key) also works.
	m = key(m, 'l')
	m = keyType(m, tea.KeyDelete)
	if len(m.ideas[0].Children) != 0 {
		t.Fatalf("delete key should also remove the node, got %+v", m.ideas[0].Children)
	}

	m = keyType(m, tea.KeyEsc) // leave the map
	if m.mode != modeIdeaList {
		t.Fatalf("esc from the map should return to the idea list, mode=%d", m.mode)
	}
}

func TestMindMapCollapseExpand(t *testing.T) {
	m := mindModel(t)
	// Build root → A → A1 directly in the model, then open.
	a1 := &MindNode{Text: "A1"}
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{a1}}}
	m = keyType(m, tea.KeyEnter)

	if len(m.mindRows()) != 3 {
		t.Fatalf("expanded tree should flatten to root+A+A1 = 3 rows, got %d", len(m.mindRows()))
	}
	// Descend to A (right), then collapse with space.
	m = keyType(m, tea.KeyRight)
	m = key(m, ' ')
	if len(m.mindRows()) != 2 {
		t.Fatalf("collapsing A should hide A1, expected 2 rows, got %d", len(m.mindRows()))
	}
	// Space again re-expands.
	m = key(m, ' ')
	if len(m.mindRows()) != 3 {
		t.Fatalf("expanding A should reveal A1 again, got %d rows", len(m.mindRows()))
	}
}

func TestMindMapLeftDoesNotCollapse(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{{Text: "A1"}}}}
	m = keyType(m, tea.KeyEnter)

	m = keyType(m, tea.KeyRight) // descend onto A (a node with children)
	m = keyType(m, tea.KeyLeft)  // back to the root, must NOT collapse A
	if len(m.mindRows()) != 3 {
		t.Fatalf("left should navigate, not collapse — expected 3 rows, got %d", len(m.mindRows()))
	}
	if m.mindCursor != 0 {
		t.Fatalf("left from A should move the cursor to the root, got %d", m.mindCursor)
	}
}

func TestMindMapSiblingNavigation(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A"}, {Text: "B"}, {Text: "C"}}
	m = keyType(m, tea.KeyEnter)

	m = keyType(m, tea.KeyRight) // root → first child A
	rows := m.mindRows()
	if rows[m.mindCursor].node.Text != "A" {
		t.Fatalf("right from root should land on A, got %q", rows[m.mindCursor].node.Text)
	}
	m = key(m, 'j') // next sibling B
	if m.mindRows()[m.mindCursor].node.Text != "B" {
		t.Fatalf("down should move to sibling B, got %q", m.mindRows()[m.mindCursor].node.Text)
	}
	m = key(m, 'j') // next sibling C
	if m.mindRows()[m.mindCursor].node.Text != "C" {
		t.Fatalf("down should move to sibling C, got %q", m.mindRows()[m.mindCursor].node.Text)
	}
	m = key(m, 'k') // back up to B
	if m.mindRows()[m.mindCursor].node.Text != "B" {
		t.Fatalf("up should move back to sibling B, got %q", m.mindRows()[m.mindCursor].node.Text)
	}
}

func TestMindMapColorCycle(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{{Text: "A1"}}}}
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyRight) // onto A

	m = key(m, 'o') // outline colour, this node only
	if m.ideas[0].Children[0].Color != 1 {
		t.Fatalf("o should set outline colour index 1, got %d", m.ideas[0].Children[0].Color)
	}
	if m.ideas[0].Children[0].Children[0].Color != 0 {
		t.Fatal("o must not touch the child's colour")
	}
	m = key(m, 'O') // outline colour for node + descendants
	if got := m.ideas[0].Children[0].Color; got != 2 {
		t.Fatalf("O should advance the node's colour to 2, got %d", got)
	}
	if got := m.ideas[0].Children[0].Children[0].Color; got != 2 {
		t.Fatalf("O should propagate colour 2 to the child, got %d", got)
	}
	m = key(m, 'f') // font colour, this node only
	if m.ideas[0].Children[0].FG != 1 {
		t.Fatalf("f should set font colour index 1, got %d", m.ideas[0].Children[0].FG)
	}
	m = key(m, 'g') // background fill, this node only
	if m.ideas[0].Children[0].BG != 1 {
		t.Fatalf("g should set background index 1, got %d", m.ideas[0].Children[0].BG)
	}
	m = key(m, 'G') // background for node + descendants
	if got := m.ideas[0].Children[0].Children[0].BG; got != 2 {
		t.Fatalf("G should propagate background 2 to the child, got %d", got)
	}
}

func TestMindMapFontColourAndStyle(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A"}}
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l') // select A

	// f cycles the font colour.
	m = key(m, 'f')
	if m.ideas[0].Children[0].FG != 1 {
		t.Fatalf("f should set font colour index 1, got %d", m.ideas[0].Children[0].FG)
	}
	// y cycles the text style: normal → bold → italic → underline → normal.
	for i, want := range []int{1, 2, 3, 0} {
		m = key(m, 'y')
		if m.ideas[0].Children[0].Style != want {
			t.Fatalf("y press %d: style = %d, want %d", i+1, m.ideas[0].Children[0].Style, want)
		}
	}
}

func TestMindMapStyleSubtree(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{{Text: "A1"}}}}
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'l') // select A

	// Y applies the style to the node AND its children.
	m = key(m, 'Y') // → bold (1)
	if m.ideas[0].Children[0].Style != 1 || m.ideas[0].Children[0].Children[0].Style != 1 {
		t.Fatalf("Y should set style on node + child, got %+v", m.ideas[0].Children[0])
	}
}

func TestMindMapSyncTopsUpBoundIdea(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Work"}
	m.deriveAll()
	m.ideas = []Idea{{Text: "Plan", ProjectID: "p", ProjectName: "#Work", Children: []*MindNode{
		{Text: "Old", IsTask: true, TaskID: "real-old"}, // already linked
		{Text: "New", IsTask: true},                     // needs creating
	}}}
	m.mindIdea, m.mode = 0, modeMindMap

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = nm.(model)

	if m.ideas[0].Children[1].TaskID == "" {
		t.Fatal("sync should create + link the new task node")
	}
	if m.ideas[0].Children[0].TaskID != "real-old" {
		t.Fatal("sync must not touch an already-linked node (no recreate)")
	}
	adds := 0
	for _, c := range m.queue {
		if c.Type == "item_add" {
			adds++
		}
	}
	if adds != 1 {
		t.Fatalf("exactly one new task should be queued, got %d", adds)
	}
	if !m.syncing || cmd == nil {
		t.Fatal("s should also start the sync")
	}
}

func TestMindMapDownPrefersSameColumn(t *testing.T) {
	m := mindModel(t)
	// sdf (depth 2) has a deep right-side chain; dfsd (depth 2) sits in a lower
	// subtree. Down from sdf must land on dfsd (same column), not a deep node.
	fdfs := &MindNode{Text: "fdfs", Children: []*MindNode{{Text: "a"}, {Text: "b"}, {Text: "c"}}}
	sdf := &MindNode{Text: "sdf", Children: []*MindNode{{Text: "sdfsd", Children: []*MindNode{fdfs}}}}
	d := &MindNode{Text: "d", Children: []*MindNode{{Text: "sdfsdf"}, sdf}}
	dfsd := &MindNode{Text: "dfsd"}
	lower := &MindNode{Text: "lower", Children: []*MindNode{dfsd, {Text: "sdf2"}}}
	m.ideas[0].Children = []*MindNode{d, lower}
	m = keyType(m, tea.KeyEnter)

	m.mindCursor = m.mindIndexOf(sdf)
	m = keyType(m, tea.KeyDown)
	if got := m.mindRows()[m.mindCursor].node; got != dfsd {
		name := "<root>"
		if got != nil {
			name = got.Text
		}
		t.Fatalf("down from sdf should land on dfsd (same column), got %q", name)
	}
}

func TestIdeaRename(t *testing.T) {
	m := mindModel(t) // idea list, ideas[0].Text == "Buy a house"
	m = key(m, 'R')
	if m.mode != modeIdeaRename {
		t.Fatalf("R should open the idea rename, mode=%d", m.mode)
	}
	m.input.SetValue("Renamed Idea")
	m = keyType(m, tea.KeyEnter)
	if m.ideas[0].Text != "Renamed Idea" {
		t.Fatalf("idea should be renamed, got %q", m.ideas[0].Text)
	}
	// Empty rename keeps the previous name (≥1 char required).
	m = key(m, 'R')
	m.input.SetValue("   ")
	m = keyType(m, tea.KeyEnter)
	if m.ideas[0].Text != "Renamed Idea" {
		t.Fatalf("empty rename should keep the previous name, got %q", m.ideas[0].Text)
	}
}

func TestMindMapRenameRoot(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // into the map
	m = key(m, 'R')
	if m.mode != modeMindEdit {
		t.Fatalf("R should start editing the root, mode=%d", m.mode)
	}
	m.input.SetValue("New Root")
	m = keyType(m, tea.KeyEnter)
	if m.ideas[0].Text != "New Root" {
		t.Fatalf("R should rename the root idea, got %q", m.ideas[0].Text)
	}
}

func TestMindMapRootCannotBeDeleted(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // into the map, cursor on root
	m = key(m, 'd')
	if m.mode != modeMindMap {
		t.Fatalf("d on the root must not open the delete dialog, mode=%d", m.mode)
	}
	if len(m.ideas) != 1 {
		t.Fatal("the root idea must not be deleted from the map")
	}
}

func TestMindMapJumpToRoot(t *testing.T) {
	m := mindModel(t)
	m.ideas[0].Children = []*MindNode{{Text: "A", Children: []*MindNode{{Text: "A1"}}}}
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyRight) // onto A
	m = keyType(m, tea.KeyRight) // onto A1
	if m.mindCursor == 0 {
		t.Fatal("precondition: cursor should have moved off the root")
	}
	m = key(m, 'r') // jump back to root
	if m.mindCursor != 0 || !m.mindRows()[m.mindCursor].isRoot {
		t.Fatalf("r should jump to the root node, cursor=%d", m.mindCursor)
	}
}

func TestMindMapTopUpSelfHealsStaleProjectID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Work"}
	m.deriveAll()
	// ProjectID is stale (e.g. a temp id that changed after the project synced),
	// but the name still matches a real project.
	m.ideas = []Idea{{Text: "Plan", ProjectID: "tmp-old", ProjectName: "#Work",
		Children: []*MindNode{{Text: "New", IsTask: true}}}}

	if n := m.topUpBoundIdeas(); n != 1 {
		t.Fatalf("expected 1 task created via self-heal, got %d", n)
	}
	if m.ideas[0].ProjectID != "p" {
		t.Fatalf("stale project id should self-heal to the real id, got %q", m.ideas[0].ProjectID)
	}
	if m.ideas[0].Children[0].TaskID == "" {
		t.Fatal("the new task should be linked")
	}
}

func TestMindMapBackKey(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter) // into the map
	m = key(m, 'b')              // b = back to the idea list
	if m.mode != modeIdeaList {
		t.Fatalf("b should return to the idea list, mode=%d", m.mode)
	}
}

func TestMindMapConvertTasksToProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 120, 40
	m.cache.Projects["p1"] = apiProject{ID: "p1", Name: "Work"}
	m.deriveAll()
	m.ideas = []Idea{{Text: "Plan", At: nowStamp(), Children: []*MindNode{
		{Text: "Spec", IsTask: true},
		{Text: "Build", IsTask: true},
		{Text: "Notes"}, // not a task
	}}}
	m.mindIdea, m.mindCursor, m.mode = 0, 0, modeMindMap

	m = key(m, 'T')
	if m.mode != modeProjectPick || m.pickIntent != pickMindTasks {
		t.Fatalf("T should open the picker for tasks, mode=%d intent=%d", m.mode, m.pickIntent)
	}
	m = typeText(m, "Work")
	m = keyType(m, tea.KeyEnter)

	if m.mode != modeMindMap {
		t.Fatalf("convert should return to the map, mode=%d", m.mode)
	}
	kids := m.ideas[0].Children
	if kids[0].TaskID == "" || kids[1].TaskID == "" {
		t.Fatal("task nodes should be linked to new task ids")
	}
	if kids[2].TaskID != "" {
		t.Fatal("non-task node must not be linked")
	}
	adds := 0
	for _, c := range m.queue {
		if c.Type == "item_add" {
			adds++
		}
	}
	if adds != 2 {
		t.Fatalf("expected 2 item_add commands, got %d", adds)
	}
	// Converting again adds nothing new — the nodes are already linked.
	if got := m.ideas[0].collectMindTasks(); len(got) != 0 {
		t.Fatalf("linked tasks should not be re-collected, got %d", len(got))
	}
}

func TestMindMapBindUnbind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 120, 40
	m.cache.Projects["p1"] = apiProject{ID: "p1", Name: "Work"}
	m.deriveAll()
	m.ideas = []Idea{{Text: "Plan", At: nowStamp(), Children: []*MindNode{{Text: "Spec", IsTask: true}}}}
	m.mindIdea, m.mode = 0, modeMindMap

	// Bind via the picker.
	m = key(m, 'T')
	m = typeText(m, "Work")
	m = keyType(m, tea.KeyEnter)
	if m.ideas[0].ProjectID != "p1" {
		t.Fatalf("converting should bind the idea to the project, got %q", m.ideas[0].ProjectID)
	}
	linkedID := m.ideas[0].Children[0].TaskID
	if linkedID == "" {
		t.Fatal("task should be linked after binding")
	}

	// Pressing T again while bound must NOT reopen the picker.
	m = key(m, 'T')
	if m.mode != modeMindMap {
		t.Fatalf("T while bound should stay on the map, mode=%d", m.mode)
	}

	// U asks first; cancel leaves the binding intact.
	m = key(m, 'U')
	if m.mode != modeMindConfirmUnbind {
		t.Fatalf("U should open the unbind confirmation, mode=%d", m.mode)
	}
	m = key(m, 'n')
	if m.ideas[0].ProjectID == "" {
		t.Fatal("cancelling unbind should keep the binding")
	}
	// Confirm this time: unbinds and unlinks every generated task id.
	m = key(m, 'U')
	m = key(m, 'y')
	if m.ideas[0].ProjectID != "" {
		t.Fatal("U+y should clear the idea's project binding")
	}
	if m.ideas[0].Children[0].TaskID != "" {
		t.Fatal("U+y should unlink the generated task ids")
	}
}

func TestMindMapSyncKeyTriggersSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ideas = []Idea{{Text: "Plan", Children: []*MindNode{{Text: "a"}}}}
	m.mindIdea, m.mode = 0, modeMindMap
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = nm.(model)
	if !m.syncing || cmd == nil {
		t.Fatal("s in the map should kick off a sync")
	}
}

func TestMindMapConvertCreatesMissingProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 120, 40
	m.deriveAll()
	m.ideas = []Idea{{Text: "Plan", At: nowStamp(), Children: []*MindNode{{Text: "Spec", IsTask: true}}}}
	m.mindIdea, m.mode = 0, modeMindMap

	m = key(m, 'T')
	m = typeText(m, "Brand New")
	m = keyType(m, tea.KeyEnter)

	if !m.projectExists("Brand New") {
		t.Fatal("typing a new project name should auto-create it")
	}
	if m.ideas[0].Children[0].TaskID == "" {
		t.Fatal("the task should be linked even when the project was just created")
	}
}

func TestMindMapSyncRemapsAndCompletes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ideas = []Idea{{Text: "root", Children: []*MindNode{{Text: "a", IsTask: true, TaskID: "tmp-1"}}}}

	resp := &syncResponse{
		TempIDMapping: map[string]string{"tmp-1": "real-1"},
		Items:         []apiItem{{ID: "real-1", Content: "a", Checked: true}},
	}
	m.cache.Merge(resp)
	m.syncMindLinks(resp.TempIDMapping)

	n := m.ideas[0].Children[0]
	if n.TaskID != "real-1" {
		t.Fatalf("sync should remap the temp id to the real id, got %q", n.TaskID)
	}
	if !n.Done {
		t.Fatal("a task completed in Todoist should mark the node done on sync")
	}
}

func TestMindMapXCompletesTaskNode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 100, 40
	m.cache.Items["real-9"] = apiItem{ID: "real-9", Content: "do"}
	m.ideas = []Idea{{Text: "root", Children: []*MindNode{{Text: "do", IsTask: true, TaskID: "real-9"}}}}
	m.mindIdea, m.mindCursor, m.mode = 0, 1, modeMindMap

	m = key(m, 'X') // complete (X = done; x is cut now)
	if !m.ideas[0].Children[0].Done {
		t.Fatal("X on a task node should complete it")
	}
	if len(m.ideas[0].Children) != 1 {
		t.Fatal("X on a task node must NOT delete it")
	}
	completes := 0
	for _, c := range m.queue {
		if c.Type == "item_complete" {
			completes++
		}
	}
	if completes != 1 {
		t.Fatalf("expected 1 item_complete queued, got %d", completes)
	}

	m = key(m, 'X') // reopen
	if m.ideas[0].Children[0].Done {
		t.Fatal("X again should reopen the task")
	}
}

func TestMindMapHelpOpens(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter)
	m = key(m, 'H')
	if m.mode != modeMindHelp {
		t.Fatalf("H should open the mind-map help, mode=%d", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "Mind-map keys") {
		t.Fatalf("help view should render its title, got:\n%s", v)
	}
	m = key(m, 'x') // any key closes
	if m.mode != modeMindMap {
		t.Fatal("any key should close the help back to the map")
	}
}
