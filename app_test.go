package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToTaskPriorityInversion(t *testing.T) {
	c := newCache()
	c.Projects["pr"] = apiProject{ID: "pr", Name: "Bills"}
	// API priority 4 = highest = display p1; API 1 = display p4
	hi := c.toTask(apiItem{ID: "1", Content: "urgent", Priority: 4, ProjectID: "pr", Labels: []string{"x"}})
	lo := c.toTask(apiItem{ID: "2", Content: "normal", Priority: 1})
	if hi.Priority != "p1" {
		t.Fatalf("API priority 4 should map to p1, got %s", hi.Priority)
	}
	if lo.Priority != "p4" {
		t.Fatalf("API priority 1 should map to p4, got %s", lo.Priority)
	}
	if hi.Project != "#Bills" {
		t.Fatalf("project = %q", hi.Project)
	}
	if hi.Labels != "@x" {
		t.Fatalf("labels = %q", hi.Labels)
	}
}

func TestToTaskDeadline(t *testing.T) {
	c := newCache()
	got := c.toTask(apiItem{ID: "1", Content: "x", Priority: 1, Deadline: &apiDeadline{Date: "2026-12-31"}})
	if got.Deadline != "2026-12-31" {
		t.Fatalf("deadline = %q", got.Deadline)
	}
}

func TestEvalFilterSubset(t *testing.T) {
	today := "2026-06-15"
	due := func(d string) Task { return Task{DueDate: d} }
	cases := []struct {
		expr string
		task Task
		want bool
	}{
		{"today", due("2026-06-15 09:00"), true},
		{"today", due("2026-06-16"), false},
		{"overdue", due("2026-06-10"), true},
		{"overdue", due("2026-06-20"), false},
		{"no date", due(""), true},
		{"no date", due("2026-06-15"), false},
		{"p1", Task{Priority: "p1"}, true},
		{"p1", Task{Priority: "p4"}, false},
		{"@ongoing", Task{Labels: "@ongoing @x"}, true},
		{"@ongoing", Task{Labels: "@x"}, false},
		{"today | overdue", due("2026-06-10"), true},
		{"recurring & p1", Task{Recurring: true, Priority: "p1"}, true},
		{"recurring & p1", Task{Recurring: false, Priority: "p1"}, false},
		{"!today", due("2026-06-16"), true},
	}
	for _, c := range cases {
		if got := EvalFilter(c.expr, c.task, today); got != c.want {
			t.Errorf("EvalFilter(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestParseQuickAdd(t *testing.T) {
	q := parseQuickAdd("Buy milk @errand tomorrow 9am p1")
	if q.Content != "Buy milk" {
		t.Fatalf("content = %q", q.Content)
	}
	if len(q.Labels) != 1 || q.Labels[0] != "errand" {
		t.Fatalf("labels = %v", q.Labels)
	}
	if q.Priority != 4 { // display p1 → API 4
		t.Fatalf("priority = %d, want 4", q.Priority)
	}
	if q.DueString != "tomorrow 9am" {
		t.Fatalf("due = %q", q.DueString)
	}
	// plain task with no date/labels
	q2 := parseQuickAdd("Buy groceries")
	if q2.Content != "Buy groceries" || q2.DueString != "" {
		t.Fatalf("plain parse wrong: %+v", q2)
	}
}

func sampleTasks() []Task {
	return []Task{
		{ID: "1", Priority: "p1", Project: "#Bills", Content: "Pay rent", DueDate: "2026-06-15"},
		{ID: "2", Priority: "p4", Project: "#Personal", Content: "Read a book"},
	}
}

func TestTasksLoadedPopulatesList(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)

	nm, _ := m.Update(tasksLoadedMsg{tasks: sampleTasks(), filter: ""})
	mm := nm.(model)
	if n := len(mm.list.Items()); n != 2 {
		t.Fatalf("want 2 items, got %d", n)
	}
	if mm.status != "2 tasks" {
		t.Fatalf("status = %q", mm.status)
	}
}

func TestKeyOpensProjectPicker(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if nm.(model).mode != modeProjectPick {
		t.Fatal("pressing 'a' should open the project picker")
	}
}

// selectProjItem moves the picker cursor to the (non-separator) item with the
// given name, so tests don't depend on the picker's row layout.
func selectProjItem(m *model, name string) {
	for i, it := range m.projList.Items() {
		if p, ok := it.(projItem); ok && p.kind != kindSep && p.p.Name == name {
			m.projList.Select(i)
			return
		}
	}
}

func TestProjectPickThenAdd(t *testing.T) {
	m := initialModel()
	m.recents = nil // deterministic: ignore any persisted recents
	m.width, m.height = 100, 40
	m.projList.SetSize(100, 36)
	// load some projects
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{
		{ID: "p1", Name: "#Personal"},
		{ID: "p2", Name: "#Bills Payments"},
	}})
	m = nm.(model)
	// open picker
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = nm.(model)
	if m.mode != modeProjectPick {
		t.Fatal("expected project pick mode")
	}
	// select #Personal
	selectProjItem(&m, "#Personal")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeAdd {
		t.Fatal("selecting a project should move to add mode")
	}
	if m.addProject.ID != "p1" {
		t.Fatalf("addProject = %q, want p1", m.addProject.ID)
	}
	if len(m.recents) == 0 || m.recents[0].ID != "p1" {
		t.Fatalf("most recent project should be p1, got %+v", m.recents)
	}
}

func TestAddToLastProjectShortcut(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.recents = []Project{{ID: "p9", Name: "#Work"}}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	mm := nm.(model)
	if mm.mode != modeAdd {
		t.Fatal("'A' with a last project should jump straight to add mode")
	}
	if mm.addProject.ID != "p9" {
		t.Fatalf("addProject = %q, want p9", mm.addProject.ID)
	}
}

func TestKeyOpensSearchMode(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if nm.(model).mode != modeSearch {
		t.Fatal("pressing '/' should enter search mode")
	}
}

func TestSearchEnterSetsFilter(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	// enter search mode
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = nm.(model)
	// type "today" (a filter expression)
	for _, r := range "today" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	// submit
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeList {
		t.Fatal("enter should return to list mode")
	}
	if m.filter != "today" {
		t.Fatalf("filter = %q, want today", m.filter)
	}
	if cmd != nil {
		t.Fatal("filters are evaluated locally now; no reload command expected")
	}
}

func TestIsFilterExpr(t *testing.T) {
	filters := []string{"today", "today | overdue", "#Personal", "@label", "p1", "7 days", "no date", "overdue & p1"}
	for _, f := range filters {
		if !isFilterExpr(f) {
			t.Errorf("%q should be detected as a filter expression", f)
		}
	}
	texts := []string{"groceries", "call mom", "report", "gym session", "buy milk"}
	for _, s := range texts {
		if isFilterExpr(s) {
			t.Errorf("%q should be treated as plain text search", s)
		}
	}
}

func TestLocalTextSearchFiltersTasks(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	tasks := []Task{
		{ID: "1", Priority: "p4", Project: "#Bills", Content: "Submit quarterly report"},
		{ID: "2", Priority: "p4", Project: "#Bills", Content: "Review report draft"},
		{ID: "3", Priority: "p4", Project: "#Personal", Content: "Read a book"},
	}
	nm, _ := m.Update(tasksLoadedMsg{tasks: tasks})
	m = nm.(model)
	if len(m.list.Items()) != 3 {
		t.Fatalf("want 3 items initially, got %d", len(m.list.Items()))
	}
	// open search and type a plain word
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = nm.(model)
	for _, r := range "report" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	// live preview should already narrow to the 2 matching tasks
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("live text search: want 2 items, got %d", got)
	}
	// submit (enter) — stays local, no server reload
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeList {
		t.Fatal("enter should return to list mode")
	}
	if m.textQuery != "report" {
		t.Fatalf("textQuery = %q", m.textQuery)
	}
	if m.filter != "" {
		t.Fatalf("plain text search must not set a server filter, got %q", m.filter)
	}
	if cmd != nil {
		t.Fatal("plain text search should not trigger a server reload")
	}
	if len(m.list.Items()) != 2 {
		t.Fatalf("want 2 matched items, got %d", len(m.list.Items()))
	}
}

func TestViewByProject(t *testing.T) {
	m := initialModel()
	m.recents = nil // deterministic
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.projList.SetSize(100, 36)
	// projects + tasks
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{
		{ID: "b", Name: "#Bills"},
		{ID: "p", Name: "#Personal"},
	}})
	m = nm.(model)
	nm, _ = m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Project: "#Bills", Content: "Pay rent"},
		{ID: "2", Project: "#Bills", Content: "Pay utilities"},
		{ID: "3", Project: "#Personal", Content: "Read a book"},
	}})
	m = nm.(model)
	// open view-by-project and choose #Bills
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	if m.mode != modeProjectPick || m.pickIntent != pickView {
		t.Fatal("'p' should open the picker in view intent")
	}
	selectProjItem(&m, "#Bills")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeList {
		t.Fatal("selecting should return to list")
	}
	if m.projectView != "#Bills" {
		t.Fatalf("projectView = %q, want #Bills", m.projectView)
	}
	if cmd != nil {
		t.Fatal("view-by-project is local; should not reload from server")
	}
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("want 2 #Bills tasks, got %d", got)
	}
	// choosing a project to view records it as recent
	if len(m.recents) == 0 || m.recents[0].Name != "#Bills" {
		t.Fatalf("view should record recent project, got %+v", m.recents)
	}

	// Reopen and choose "All Projects" to go back.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	selectProjItem(&m, "↩ All Projects")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.projectView != "" {
		t.Fatalf("selecting All Projects should clear projectView, got %q", m.projectView)
	}
	if got := len(m.list.Items()); got != 3 {
		t.Fatalf("All Projects should show all 3 tasks, got %d", got)
	}

	// Re-apply a project view, then verify Esc also clears it.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	selectProjItem(&m, "#Bills")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.projectView != "#Bills" {
		t.Fatalf("expected #Bills view, got %q", m.projectView)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.projectView != "" {
		t.Fatal("esc should clear projectView")
	}
	if got := len(m.list.Items()); got != 3 {
		t.Fatalf("after clear want 3 tasks, got %d", got)
	}
}

func TestBackAndHomeNavigation(t *testing.T) {
	m := initialModel()
	m.recents = nil
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.projList.SetSize(100, 36)
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{{ID: "b", Name: "#Bills"}}})
	m = nm.(model)
	nm, _ = m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Project: "#Bills", Content: "Pay rent report"},
		{ID: "2", Project: "#Bills", Content: "Pay utilities"},
		{ID: "3", Project: "#Personal", Content: "Read a book"},
	}})
	m = nm.(model)

	// View #Bills
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	selectProjItem(&m, "#Bills")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.projectView != "#Bills" || len(m.list.Items()) != 2 {
		t.Fatalf("expected #Bills view with 2 items, got %q/%d", m.projectView, len(m.list.Items()))
	}

	// Then search "report" within it
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = nm.(model)
	for _, r := range "report" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.textQuery != "report" || m.projectView != "#Bills" || len(m.list.Items()) != 1 {
		t.Fatalf("search-in-project: got query=%q project=%q items=%d", m.textQuery, m.projectView, len(m.list.Items()))
	}

	// Back → returns to #Bills view (no search)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = nm.(model)
	if m.textQuery != "" || m.projectView != "#Bills" || len(m.list.Items()) != 2 {
		t.Fatalf("after back: query=%q project=%q items=%d", m.textQuery, m.projectView, len(m.list.Items()))
	}

	// Back again → all tasks
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = nm.(model)
	if m.projectView != "" || len(m.list.Items()) != 3 {
		t.Fatalf("after second back: project=%q items=%d", m.projectView, len(m.list.Items()))
	}

	// Home from a project view jumps straight to all tasks
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	selectProjItem(&m, "#Bills")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = nm.(model)
	if m.projectView != "" || len(m.list.Items()) != 3 {
		t.Fatalf("home: project=%q items=%d", m.projectView, len(m.list.Items()))
	}
}

func TestProjectPickerTypeToFilter(t *testing.T) {
	m := initialModel()
	m.recents = nil
	m.width, m.height = 100, 40
	m.projList.SetSize(100, 36)
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{
		{ID: "1", Name: "#Personal"},
		{ID: "2", Name: "#Bills"},
		{ID: "3", Name: "#Bizlink"},
	}})
	m = nm.(model)
	// open the picker
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	// type "biz" → only #Bizlink matches
	for _, r := range "biz" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	if m.projQuery != "biz" {
		t.Fatalf("projQuery = %q", m.projQuery)
	}
	if got := len(m.projList.Items()); got != 1 {
		t.Fatalf("filter 'biz' should leave 1 project, got %d", got)
	}
	// backspace widens to "bi" → #Bills and #Bizlink
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = nm.(model)
	if got := len(m.projList.Items()); got != 2 {
		t.Fatalf("filter 'bi' should leave 2 projects, got %d", got)
	}
	// esc clears the filter, restoring the full list (+ All Projects in view mode)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.projQuery != "" || m.mode != modeProjectPick {
		t.Fatal("first esc should clear the filter, not close the picker")
	}
}

func TestClearDataDialogCancel(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = nm.(model)
	if m.mode != modeClearData {
		t.Fatal("X should open the clear-data confirmation")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nm.(model).mode != modeList {
		t.Fatal("n should cancel and return to the list")
	}
}

func TestTokenCheckedValidLeavesOnboard(t *testing.T) {
	m := initialModel()
	m.cache = newCache()
	m.width, m.height = 100, 40
	m.mode = modeOnboard
	nm, cmd := m.Update(tokenCheckedMsg{valid: true})
	mm := nm.(model)
	if mm.mode != modeList {
		t.Fatal("valid token should leave onboarding for the list")
	}
	if !mm.online {
		t.Fatal("valid token should mark online")
	}
	if cmd == nil {
		t.Fatal("valid token should trigger a sync")
	}
}

func TestTokenCheckedAuthErrStaysOnboard(t *testing.T) {
	m := initialModel()
	m.cache = newCache()
	m.width, m.height = 100, 40
	m.mode = modeList
	nm, _ := m.Update(tokenCheckedMsg{authErr: true})
	if nm.(model).mode != modeOnboard {
		t.Fatal("a rejected token should drop into onboarding")
	}
}

func TestHelpPageToggle(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	m = nm.(model)
	if m.mode != modeHelp {
		t.Fatal("H should open the help page")
	}
	if !strings.Contains(m.View(), "Navigation") {
		t.Fatal("help view should render its sections")
	}
	// any non-scroll key closes
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if nm.(model).mode != modeList {
		t.Fatal("any key should close help")
	}
}

func TestHelpScrolling(t *testing.T) {
	m := initialModel()
	m.width, m.height = 80, 12 // short terminal → content overflows
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	m = nm.(model)
	if m.maxHelpOffset() <= 0 {
		t.Fatal("with a short terminal the help should be scrollable")
	}
	// j scrolls down without closing
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = nm.(model)
	if m.mode != modeHelp || m.helpOffset != 1 {
		t.Fatalf("j should scroll, not close (mode=%v offset=%d)", m.mode, m.helpOffset)
	}
	// k scrolls back up
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = nm.(model)
	if m.helpOffset != 0 {
		t.Fatalf("k should scroll up, offset=%d", m.helpOffset)
	}
	// G jumps to end
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = nm.(model)
	if m.helpOffset != m.maxHelpOffset() {
		t.Fatal("G should jump to end")
	}
	// a real key (e.g. q) closes
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if nm.(model).mode != modeList {
		t.Fatal("q should close help")
	}
}

func TestEnterOpensDetail(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Priority: "p1", Project: "#Bills", Content: "Pay rent", DueDate: "today", Labels: "@ongoing"},
	}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeDetail {
		t.Fatal("enter should open the detail view, not complete")
	}
	if m.detailID != "1" {
		t.Fatalf("detailID = %q", m.detailID)
	}
	if !strings.Contains(m.View(), "Pay rent") {
		t.Fatal("detail view should render the task content")
	}
	// open the date editor
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if m.mode != modeDetailEdit || m.editField != efDate {
		t.Fatal("'t' should open the date editor")
	}
	// esc returns to detail
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.mode != modeDetail {
		t.Fatal("esc should return to detail view")
	}
	// 'b' returns to the list
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if nm.(model).mode != modeList {
		t.Fatal("'b' should return to the list")
	}
}

func TestOngoingFilter(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Content: "a", Labels: "@ongoing"},
		{ID: "2", Content: "b", Labels: "@x"},
		{ID: "3", Content: "c", Labels: "@ongoing @y"},
	}})
	m = nm.(model)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = nm.(model)
	if m.filter != "@ongoing" {
		t.Fatalf("'o' should set the @ongoing filter, got %q", m.filter)
	}
	if cmd != nil {
		t.Fatal("filters are local now; no reload command expected")
	}
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("want 2 @ongoing tasks, got %d", got)
	}
}

func TestPriorityFilter(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Priority: "p1", Content: "urgent"},
		{ID: "2", Priority: "p4", Content: "normal"},
		{ID: "3", Priority: "p1", Content: "urgent 2"},
	}})
	m = nm.(model)
	// open priority picker
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = nm.(model)
	if m.mode != modePriorityPick {
		t.Fatal("P should open the priority picker")
	}
	// pick p1 directly
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	if m.priorityView != "p1" {
		t.Fatalf("priorityView = %q, want p1", m.priorityView)
	}
	if cmd != nil {
		t.Fatal("priority filter is local; no reload expected")
	}
	if len(m.list.Items()) != 2 {
		t.Fatalf("want 2 p1 tasks, got %d", len(m.list.Items()))
	}
	// reopen and choose All priorities (0) to clear
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	m = nm.(model)
	if m.priorityView != "" {
		t.Fatal("All priorities should clear the priority filter")
	}
	if len(m.list.Items()) != 3 {
		t.Fatalf("want all 3 tasks, got %d", len(m.list.Items()))
	}
}

func TestSortByPriority(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Priority: "p4", Content: "low"},
		{ID: "2", Priority: "p1", Content: "high"},
		{ID: "3", Priority: "p2", Content: "mid"},
	}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	first := m.list.Items()[0].(taskItem).t
	if first.Priority != "p1" {
		t.Fatalf("after sort by priority, first should be p1, got %s", first.Priority)
	}
	// reverse
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	if !m.sortDesc {
		t.Fatal("pressing 1 again should reverse the sort")
	}
	if m.list.Items()[0].(taskItem).t.Priority != "p4" {
		t.Fatal("reversed priority sort should put p4 first")
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: sampleTasks()})
	m = nm.(model)
	// press d -> confirm mode
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = nm.(model)
	if m.mode != modeConfirm {
		t.Fatal("d should open confirm")
	}
	// press n -> cancel back to list
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nm.(model).mode != modeList {
		t.Fatal("n should cancel delete")
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: sampleTasks()})
	out := nm.(model).View()
	if out == "" {
		t.Fatal("view should render content")
	}
}
