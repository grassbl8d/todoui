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
	if !strings.Contains(v, "i catch") || !strings.Contains(v, "b back") || !strings.Contains(v, "h home") {
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
	m = key(m, 'h') // home → back to the task list
	if m.mode != modeList {
		t.Fatalf("h should return to the task list, mode=%d", m.mode)
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

	// Tab adds a child to the root, then we type its text and commit.
	m = keyType(m, tea.KeyTab)
	if m.mode != modeMindEdit {
		t.Fatal("tab should start editing the new child node")
	}
	m = typeText(m, "Location")
	m = keyType(m, tea.KeyEnter)

	if got := m.ideas[0].Children; len(got) != 1 || got[0].Text != "Location" {
		t.Fatalf("tab should add one child 'Location', got %+v", got)
	}

	// Enter adds a sibling of the selected node (now "Location").
	m = keyType(m, tea.KeyEnter)
	m = typeText(m, "Budget")
	m = keyType(m, tea.KeyEnter)

	kids := m.ideas[0].Children
	if len(kids) != 2 || kids[0].Text != "Location" || kids[1].Text != "Budget" {
		t.Fatalf("enter should add a sibling 'Budget' after 'Location', got %+v", kids)
	}

	// Tab on "Budget" nests a child under it.
	m = keyType(m, tea.KeyTab)
	m = typeText(m, "Under 500k")
	m = keyType(m, tea.KeyEnter)
	if gk := m.ideas[0].Children[1].Children; len(gk) != 1 || gk[0].Text != "Under 500k" {
		t.Fatalf("tab should nest a grandchild under Budget, got %+v", gk)
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

func TestMindMapMarkAsTask(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyTab)
	m = typeText(m, "Call agent")
	m = keyType(m, tea.KeyEnter) // commits; cursor lands on the new node

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

func TestMindMapDeleteAndEsc(t *testing.T) {
	m := mindModel(t)
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyTab)
	m = typeText(m, "Temp")
	m = keyType(m, tea.KeyEnter)

	m = key(m, 'x') // x on a non-task node does nothing
	if len(m.ideas[0].Children) != 1 {
		t.Fatalf("x must NOT delete a non-task node, got %+v", m.ideas[0].Children)
	}
	m = key(m, 'd') // d asks for confirmation
	if m.mode != modeMindConfirmDelete {
		t.Fatalf("d should open the delete confirmation, mode=%d", m.mode)
	}
	m = key(m, 'n') // cancel — node stays
	if len(m.ideas[0].Children) != 1 || m.mode != modeMindMap {
		t.Fatalf("n should cancel the delete, got %+v mode=%d", m.ideas[0].Children, m.mode)
	}
	m = key(m, 'd') // confirm this time
	m = key(m, 'y')
	if len(m.ideas[0].Children) != 0 {
		t.Fatalf("y should confirm the delete, got %+v", m.ideas[0].Children)
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

	m = key(m, 'c') // outline colour, this node only
	if m.ideas[0].Children[0].Color != 1 {
		t.Fatalf("c should set outline colour index 1, got %d", m.ideas[0].Children[0].Color)
	}
	if m.ideas[0].Children[0].Children[0].Color != 0 {
		t.Fatal("c must not touch the child's colour")
	}
	m = key(m, 'C') // outline colour for node + descendants
	if got := m.ideas[0].Children[0].Color; got != 2 {
		t.Fatalf("C should advance the node's colour to 2, got %d", got)
	}
	if got := m.ideas[0].Children[0].Children[0].Color; got != 2 {
		t.Fatalf("C should propagate colour 2 to the child, got %d", got)
	}
	m = key(m, 'v') // background, this node only
	if m.ideas[0].Children[0].BG != 1 {
		t.Fatalf("v should set background index 1, got %d", m.ideas[0].Children[0].BG)
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

	m = key(m, 'x') // complete (not delete — it's a task)
	if !m.ideas[0].Children[0].Done {
		t.Fatal("x on a task node should complete it")
	}
	if len(m.ideas[0].Children) != 1 {
		t.Fatal("x on a task node must NOT delete it")
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

	m = key(m, 'x') // reopen
	if m.ideas[0].Children[0].Done {
		t.Fatal("x again should reopen the task")
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
