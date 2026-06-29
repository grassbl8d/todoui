package todoui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMain isolates all on-disk state (cache, queue, ideas, settings, …) to a
// temp HOME so the suite never touches the real ~/.config/todo-ui.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "todoui-test-home")
	if err == nil {
		os.Setenv("HOME", tmp)
		defer os.RemoveAll(tmp)
	}
	os.Exit(m.Run())
}

// newTestModel builds a model with persisted state (pin/recents/cache) neutralized
// so tests are deterministic regardless of the dev machine's ~/.config/todo-ui.
func newTestModel() model {
	m := initialModel()
	m.pinnedID = ""
	m.recents = nil
	m.cache = newCache()
	m.queue = nil
	m.mode = modeList
	m.detailID = ""
	return m
}

func TestSortSubheadersByDateAdded(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	// Two tasks added on different dates → two date subheaders (default sort is
	// date-added descending).
	m.cache.Items["a"] = apiItem{ID: "a", Content: "newest", Priority: 1, AddedAt: "2026-06-17T00:00:00Z"}
	m.cache.Items["b"] = apiItem{ID: "b", Content: "older", Priority: 1, AddedAt: "2026-06-15T00:00:00Z"}
	m.deriveAll()

	hdrs, tasks := 0, 0
	for _, it := range m.list.Items() {
		ti := it.(taskItem)
		switch {
		case ti.hdr:
			hdrs++
		case !ti.sep:
			tasks++
		}
	}
	if hdrs != 2 || tasks != 2 {
		t.Fatalf("expected 2 subheaders + 2 tasks, got hdrs=%d tasks=%d", hdrs, tasks)
	}
	// The first row is a subheader, so the cursor must have snapped to a task.
	if it, ok := m.list.SelectedItem().(taskItem); !ok || it.sep {
		t.Fatalf("cursor should not start on a subheader, got %+v", m.list.SelectedItem())
	}

	// All in one group → no subheaders.
	m2 := newTestModel()
	m2.width, m2.height = 100, 40
	m2.list.SetSize(100, 36)
	m2.cache.Items["a"] = apiItem{ID: "a", Content: "x", Priority: 1, AddedAt: "2026-06-17T00:00:00Z"}
	m2.cache.Items["b"] = apiItem{ID: "b", Content: "y", Priority: 1, AddedAt: "2026-06-17T00:00:00Z"}
	m2.deriveAll()
	for _, it := range m2.list.Items() {
		if it.(taskItem).hdr {
			t.Fatal("a single group should produce no subheaders")
		}
	}
}

// firstTask returns the first selectable task row, skipping any subheaders.
func firstTask(m model) Task {
	for _, it := range m.list.Items() {
		if ti, ok := it.(taskItem); ok && !ti.sep {
			return ti.t
		}
	}
	return Task{}
}

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

func TestDateFormatHelpers(t *testing.T) {
	defer func() { dateFmt = "MDY" }()
	dateFmt = "MDY"
	if got := fmtDate("2026-12-31"); got != "12-31-2026" {
		t.Fatalf("MDY fmtDate = %q", got)
	}
	if got := fmtDate("2026-12-31 09:00"); got != "12-31-2026 09:00" {
		t.Fatalf("MDY fmtDate with time = %q", got)
	}
	if got := normalizeDateInput("12-31-2026"); got != "2026-12-31" {
		t.Fatalf("MDY normalize = %q", got)
	}
	dateFmt = "DMY"
	if got := fmtDate("2026-12-31"); got != "31-12-2026" {
		t.Fatalf("DMY fmtDate = %q", got)
	}
	if got := normalizeDateInput("31/12/2026"); got != "2026-12-31" {
		t.Fatalf("DMY normalize = %q", got)
	}
	dateFmt = "YMD"
	if got := fmtDate("2026-12-31"); got != "2026-12-31" {
		t.Fatalf("YMD fmtDate = %q", got)
	}
	// natural language passes through normalize untouched
	if got := normalizeDateInput("next week"); got != "next week" {
		t.Fatalf("NL normalize should pass through, got %q", got)
	}
}

func TestParseHumanDate(t *testing.T) {
	today := "2026-06-17" // Wednesday
	cases := map[string]string{
		"today":      "2026-06-17",
		"tomorrow":   "2026-06-18",
		"next week":  "2026-06-24",
		"next month": "2026-07-17",
		"2026-08-01": "2026-08-01",
		"friday":     "2026-06-19",
		"in 3 days":  "2026-06-20",
	}
	for in, want := range cases {
		if got := parseHumanDate(in, today); got != want {
			t.Errorf("parseHumanDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeadlinePicker(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "task", Priority: 1}
	m.deriveAll()
	// open detail, then deadline picker
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	m = nm.(model)
	if m.mode != modeDeadlinePick {
		t.Fatal("D should open the deadline picker")
	}
	// press 1 → Today
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	if got := m.cache.Items["1"].Deadline; got == nil || got.Date != todayStr() {
		t.Fatalf("selecting Today should set deadline to today, got %+v", got)
	}
}

func TestIdeasHotkeyFromDetail(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "task", Priority: 1}
	m.deriveAll()
	// open the detail view
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeDetail {
		t.Fatalf("enter should open detail, mode=%d", m.mode)
	}
	// I opens the ideas list from the detail view (global hotkey)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("I")})
	m = nm.(model)
	if m.mode != modeIdeaList {
		t.Fatalf("I should open the ideas list from detail, mode=%d", m.mode)
	}
}

func TestDefaultSortMenuCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the settings.Save() this test triggers
	m := newTestModel()
	m.settings.DefaultSort = "added-desc"
	// Open the menu and move the cursor onto the "Default sort" row (index 6).
	m.mode = modeOptions
	m.optCursor = 6
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// added-desc → next in the cycle is priority-asc.
	if m.settings.DefaultSort != "priority-asc" {
		t.Fatalf("cycling from added-desc should give priority-asc, got %q", m.settings.DefaultSort)
	}
	if m.sortMode != sortPriority || m.sortDesc {
		t.Fatalf("live sort should follow the menu: mode=%v desc=%v", m.sortMode, m.sortDesc)
	}
}

func TestMindUnderlineMenuCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate the settings.Save() this test triggers
	m := newTestModel()
	m.settings.MindUnderline = "yellow"
	mindUnderlineColor = mindUnderlineColorByName("yellow")
	// Open the menu and move onto the "Mind-map underline" row (index 7).
	m.mode = modeOptions
	m.optCursor = 7
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	want := nextMindUnderline("yellow")
	if m.settings.MindUnderline != want {
		t.Fatalf("cycling from yellow should give %q, got %q", want, m.settings.MindUnderline)
	}
	if mindUnderlineColor != mindUnderlineColorByName(want) {
		t.Fatalf("active underline colour should follow the menu")
	}
}

func TestStatusTimeoutMenuCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.settings.StatusSeconds = 5
	// Open the menu and move onto the "Status auto-clear" row (index 8).
	m.mode = modeOptions
	m.optCursor = 8
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.settings.StatusSeconds != 10 { // 5 → 10 in the cycle
		t.Fatalf("cycling from 5 should give 10, got %d", m.settings.StatusSeconds)
	}
	// Cycle through to the "off" (-1) entry.
	for i := 0; i < 2; i++ { // 10 → 30 → -1
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = nm.(model)
	}
	if m.settings.StatusSeconds != -1 {
		t.Fatalf("expected the off sentinel (-1), got %d", m.settings.StatusSeconds)
	}
	if statusSecondsLabel(m.settings.StatusSeconds) != "off" {
		t.Fatalf("off should render as 'off', got %q", statusSecondsLabel(m.settings.StatusSeconds))
	}
	// When off, no auto-clear timer is scheduled.
	m.status = "synced"
	if cmd := m.flashStatusCmd(); cmd != nil {
		t.Fatal("flashStatusCmd should be nil when auto-clear is off")
	}
}

func TestParseDefaultSortRoundTrip(t *testing.T) {
	cases := []struct {
		tok  string
		sm   sortMode
		desc bool
	}{
		{"added-desc", sortAdded, true},
		{"priority-asc", sortPriority, false},
		{"none", sortNone, false},
		{"", sortAdded, true},           // empty → default
		{"bogus-desc", sortAdded, true}, // unknown → default
	}
	for _, c := range cases {
		sm, desc := parseDefaultSort(c.tok)
		if sm != c.sm || desc != c.desc {
			t.Errorf("parseDefaultSort(%q) = (%v,%v), want (%v,%v)", c.tok, sm, desc, c.sm, c.desc)
		}
	}
	if got := formatDefaultSort(sortAdded, true); got != "added-desc" {
		t.Fatalf("formatDefaultSort = %q", got)
	}
}

func TestHeaderShowsIdeasAffordance(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 120, 40
	m.list.SetSize(120, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "task", Priority: 1}
	m.deriveAll()
	if v := m.View(); !strings.Contains(v, "💡 I") {
		t.Fatalf("task-list header should advertise the ideas affordance (💡 I), got:\n%s", v)
	}
}

func TestDuePicker(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "task", Priority: 1}
	m.deriveAll()
	// open detail, then the due-date picker
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if m.mode != modeDuePick {
		t.Fatal("t should open the due-date picker")
	}
	// press 1 → Today: due is stored as the natural-language phrase.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	if got := m.cache.Items["1"].Due; got == nil || got.String != "today" {
		t.Fatalf("selecting Today should set due string to \"today\", got %+v", got)
	}
	if m.mode != modeDetail {
		t.Fatalf("after picking, mode should return to detail, got %v", m.mode)
	}

	// a recurring pick keeps its phrase (so the schedule survives).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")}) // "Every Monday"
	m = nm.(model)
	if got := m.cache.Items["1"].Due; got == nil || got.String != "every monday" {
		t.Fatalf("recurring pick should set due to \"every monday\", got %+v", got)
	}
}

func TestHomeFlash(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	m = nm.(model)
	if !m.homeFlash {
		t.Fatal("h should set the home flash")
	}
	if cmd == nil {
		t.Fatal("h should schedule the flash-off tick")
	}
	nm, _ = m.Update(homeFlashOffMsg{})
	if nm.(model).homeFlash {
		t.Fatal("flash-off should clear the flash")
	}
}

func TestToTaskDeadline(t *testing.T) {
	c := newCache()
	got := c.toTask(apiItem{ID: "1", Content: "x", Priority: 1, Deadline: &apiDeadline{Date: "2026-12-31"}})
	if got.Deadline != "2026-12-31" {
		t.Fatalf("deadline = %q", got.Deadline)
	}
}

func TestEvalFilterWeekMonth(t *testing.T) {
	today := "2026-06-17" // a Wednesday
	due := func(d string) Task { return Task{DueDate: d} }
	cases := []struct {
		expr string
		date string
		want bool
	}{
		{"this week", "2026-06-15", true},  // Monday this week
		{"this week", "2026-06-21", true},  // Sunday this week
		{"this week", "2026-06-10", true},  // last week
		{"this week", "2026-06-07", false}, // two weeks ago
		{"this month", "2026-06-01", true},
		{"this month", "2026-06-30", true},
		{"this month", "2026-05-31", false},      // last month excluded
		{"this+last month", "2026-05-15", true},  // last month included
		{"this+last month", "2026-06-15", true},  // this month
		{"this+last month", "2026-04-30", false}, // two months ago
	}
	for _, c := range cases {
		if got := EvalFilter(c.expr, due(c.date), today); got != c.want {
			t.Errorf("EvalFilter(%q, due=%s) = %v, want %v", c.expr, c.date, got, c.want)
		}
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

	// "on"/"in"/"next"/"every" used as ordinary English must NOT be treated as
	// a due date unless followed by a date-like word.
	noDue := []string{
		"Remind to add this on some features",
		"Work on the dashboard",
		"Investigate issue when on word confuses the parser",
		"Interested in cooking lessons",
		"Plan the next big release",
		"Reward every developer",
	}
	for _, in := range noDue {
		q := parseQuickAdd(in)
		if q.DueString != "" {
			t.Errorf("parseQuickAdd(%q): unexpected due %q (content %q)", in, q.DueString, q.Content)
		}
		if q.Content != in {
			t.Errorf("parseQuickAdd(%q): content = %q, want full text", in, q.Content)
		}
	}

	// the same prepositions DO start a date phrase when followed by a date word.
	withDue := map[string]string{
		"Pay rent on monday":       "on monday",
		"Ship release on jan 5":    "on jan 5",
		"Call mom in 3 days":       "in 3 days",
		"Standup every day":        "every day",
		"Review next week":         "next week",
		"Renew cert on 2026-08-01": "on 2026-08-01",
	}
	for in, want := range withDue {
		q := parseQuickAdd(in)
		if q.DueString != want {
			t.Errorf("parseQuickAdd(%q): due = %q, want %q", in, q.DueString, want)
		}
	}
}

func sampleTasks() []Task {
	return []Task{
		{ID: "1", Priority: "p1", Project: "#Bills", Content: "Pay rent", DueDate: "2026-06-15"},
		{ID: "2", Priority: "p4", Project: "#Personal", Content: "Read a book"},
	}
}

func TestTasksLoadedPopulatesList(t *testing.T) {
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if nm.(model).mode != modeSearch {
		t.Fatal("pressing '/' should enter search mode")
	}
}

func TestSearchEnterSetsFilter(t *testing.T) {
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
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
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	m = nm.(model)
	if m.projectView != "" || len(m.list.Items()) != 3 {
		t.Fatalf("home: project=%q items=%d", m.projectView, len(m.list.Items()))
	}
}

func TestProjectPickerTypeToFilter(t *testing.T) {
	m := newTestModel()
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
	m := newTestModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = nm.(model)
	if m.mode != modeClearData {
		t.Fatal("X should open the clear-data confirmation")
	}
	view := m.View()
	if !strings.Contains(view, "Clear all local data?") || !strings.Contains(view, "clear everything") {
		t.Fatal("clear-data dialog should render its question and prompt")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nm.(model).mode != modeList {
		t.Fatal("n should cancel and return to the list")
	}
}

func TestAddWhileViewingProjectSkipsPicker(t *testing.T) {
	m := newTestModel()
	m.recents = nil
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.projList.SetSize(100, 36)
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{{ID: "b", Name: "#Bizlink API"}}})
	m = nm.(model)
	nm, _ = m.Update(tasksLoadedMsg{tasks: []Task{{ID: "1", Project: "#Bizlink API", Content: "x"}}})
	m = nm.(model)
	// view the project
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	selectProjItem(&m, "#Bizlink API")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.projectView != "#Bizlink API" {
		t.Fatalf("expected to be viewing #Bizlink API, got %q", m.projectView)
	}
	// press a → should go straight to add (no picker) with the project preset
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = nm.(model)
	if m.mode != modeAdd {
		t.Fatalf("add while viewing a project should skip the picker, mode=%v", m.mode)
	}
	if m.addProject.ID != "b" {
		t.Fatalf("add should target the viewed project, got %q", m.addProject.ID)
	}
}

func TestAddIdeaFunc(t *testing.T) {
	ideas := addIdea(nil, "first")
	ideas = addIdea(ideas, "second")
	if len(ideas) != 2 || ideas[0].Text != "second" {
		t.Fatalf("addIdea should prepend newest first, got %+v", ideas)
	}
}

func TestIdeaCatcherFlow(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	// i opens the capture overlay
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = nm.(model)
	if m.mode != modeIdeaAdd {
		t.Fatal("i should open the idea catcher")
	}
	if !strings.Contains(m.View(), "Catch an idea") {
		t.Fatal("idea overlay should render")
	}
	for _, r := range "ship it" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if len(m.ideas) != 1 || m.ideas[0].Text != "ship it" {
		t.Fatalf("idea should be captured, got %+v", m.ideas)
	}
	// After capturing, land directly on the ideas list showing the new idea.
	if m.mode != modeIdeaList || !strings.Contains(m.View(), "Ideas (1)") {
		t.Fatal("after capture should land on the ideas list")
	}
	// x asks for confirmation first (does not delete yet).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = nm.(model)
	if m.mode != modeIdeaConfirmDelete {
		t.Fatal("x should open the delete confirmation")
	}
	if len(m.ideas) != 1 {
		t.Fatal("x should not delete before confirmation")
	}
	if !strings.Contains(m.View(), "Delete idea?") {
		t.Fatal("confirmation modal should render")
	}
	// n cancels — the idea survives.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = nm.(model)
	if m.mode != modeIdeaList || len(m.ideas) != 1 {
		t.Fatal("n should cancel and keep the idea")
	}
	// x then y confirms the delete.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = nm.(model)
	if len(m.ideas) != 0 {
		t.Fatal("y should delete the selected idea")
	}
	if m.mode != modeIdeaList {
		t.Fatal("after delete should return to the ideas list")
	}
}

func TestIdeaCatcherWorksWhilePinned(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "focus", Priority: 1}
	m.pinnedID = "1"
	m.deriveAll()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if nm.(model).mode != modeIdeaAdd {
		t.Fatal("i should open the idea catcher even while pinned")
	}
}

func TestProjectAddDeleteOpen(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.projList.SetSize(100, 36)
	nm, _ := m.Update(projectsLoadedMsg{projects: []Project{{ID: "1", Name: "#Work"}}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = nm.(model)
	// ctrl+n opens new-project input
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	if m.mode != modeProjectAdd {
		t.Fatal("ctrl+n should open the new-project input")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.mode != modeProjectPick {
		t.Fatal("esc should return to the picker")
	}
	// ctrl+d on a project opens delete confirm
	selectProjItem(&m, "#Work")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = nm.(model)
	if m.mode != modeProjectDelete || m.projDelTarget.Name != "#Work" {
		t.Fatal("ctrl+d should open the delete confirm for the selected project")
	}
}

func TestThemeToggle(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	start := m.settings.Light
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = nm.(model)
	if m.settings.Light == start {
		t.Fatal("+ should toggle the light/dark setting")
	}
	// applyTheme should have switched the active palette
	if m.settings.Light && brandRed != lightTheme.Accent {
		t.Fatal("light theme not applied")
	}
	if !m.settings.Light && brandRed != darkTheme.Accent {
		t.Fatal("dark theme not applied")
	}
	// restore dark for other tests
	applyTheme(darkTheme)
}

func TestPinFocusAndUnpin(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Content: "focus task"},
		{ID: "2", Content: "other a"},
		{ID: "3", Content: "other b"},
	}})
	m = nm.(model)
	// select the 2nd task, then pin it
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("^")})
	m = nm.(model)
	if m.pinnedID != "2" {
		t.Fatalf("^ should pin the selected task, got %q", m.pinnedID)
	}
	if len(m.list.Items()) != 1 {
		t.Fatalf("pinned: only 1 task should show, got %d", len(m.list.Items()))
	}
	// view-switching is blocked while pinned
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if m.filter != "" || len(m.list.Items()) != 1 {
		t.Fatal("view-switch keys should be blocked while pinned")
	}
	// focus screen is shown with the unpin instruction
	v := m.View()
	if !strings.Contains(v, "P I N N E D") || !strings.Contains(v, "unpin") {
		t.Fatal("pinned focus screen should be visible with unpin instructions")
	}
	// :unpin releases
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = nm.(model)
	if m.mode != modeCommand {
		t.Fatal(": should open the command line")
	}
	for _, r := range "unpin" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.pinnedID != "" {
		t.Fatal(":unpin should clear the pin")
	}
	if len(m.list.Items()) != 3 {
		t.Fatalf("after unpin all 3 tasks should show, got %d", len(m.list.Items()))
	}
}

func TestPinFromDetail(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Content: "a"}, {ID: "2", Content: "b"},
	}})
	m = nm.(model)
	// open detail on the first task, then pin it
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeDetail {
		t.Fatal("enter should open detail")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("^")})
	m = nm.(model)
	if m.pinnedID != "1" {
		t.Fatalf("^ in detail should pin the task, got %q", m.pinnedID)
	}
	if m.mode != modeList {
		t.Fatal("pinning from detail should return to the focus view")
	}
}

func TestPinnedAddCommentShowsComments(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "focus", Priority: 1}
	m.pinnedID = "1"
	m.deriveAll()
	// press > to add a comment
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	m = nm.(model)
	if m.mode != modeCommentAdd {
		t.Fatal("> should open the comment input while pinned")
	}
	for _, r := range "looks good" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.showComments {
		t.Fatal("adding a comment should auto-show comments")
	}
	if got := len(m.cache.CommentsFor("1")); got != 1 {
		t.Fatalf("comment should be queued/cached, got %d", got)
	}
	if !strings.Contains(m.View(), "Comments (1)") {
		t.Fatal("pinned card should list the new comment")
	}
	// v toggles comments off
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if nm.(model).showComments {
		t.Fatal("v should toggle comments off")
	}
}

func TestCompletingPinnedAutoUnpins(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "focus", Priority: 1}
	m.pinnedID = "1"
	m.deriveAll()
	m.completeTask("1", "focus")
	if m.pinnedID != "" {
		t.Fatal("completing the pinned task should auto-unpin")
	}
}

func TestOnlineSearchResults(t *testing.T) {
	m := newTestModel()
	m.cache = newCache()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	// open online search
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = nm.(model)
	if m.mode != modeOnlineSearch {
		t.Fatal("'?' should open online search")
	}
	m.mode = modeList // enter would have returned to the list before results arrive
	// deliver results (as if the API returned them)
	nm, _ = m.Update(onlineResultMsg{query: "today", items: []apiItem{
		{ID: "1", Content: "due today", Priority: 1},
		{ID: "2", Content: "also today", Priority: 1},
	}})
	m = nm.(model)
	if !m.onlineView {
		t.Fatal("results should switch to the online view")
	}
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("want 2 online results, got %d", got)
	}
	// home leaves the online view
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if nm.(model).onlineView {
		t.Fatal("home should leave the online view")
	}
}

func TestMenuPageOpens(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	m = nm.(model)
	if m.mode != modeOptions {
		t.Fatal(", should open the menu")
	}
	if !strings.Contains(m.View(), "Menu") {
		t.Fatal("menu page should render")
	}
}

func TestTagOngoingFollowup(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	m.cache.Items["1"] = apiItem{ID: "1", Content: "task", Priority: 1}
	m.deriveAll()
	// O adds the ongoing label
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("O")})
	m = nm.(model)
	if got := m.cache.Items["1"].Labels; len(got) != 1 || got[0] != m.settings.OngoingLabel {
		t.Fatalf("O should add the ongoing label, got %v", got)
	}
	// O again removes it (toggle)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("O")})
	m = nm.(model)
	if len(m.cache.Items["1"].Labels) != 0 {
		t.Fatal("O again should remove the ongoing label")
	}
	// F adds the follow-up label
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = nm.(model)
	if got := m.cache.Items["1"].Labels; len(got) != 1 || got[0] != m.settings.FollowupLabel {
		t.Fatalf("F should add the follow-up label, got %v", got)
	}
	// U adds the up-next label
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = nm.(model)
	if got := m.cache.Items["1"].Labels; len(got) != 2 || got[1] != m.settings.UpNextLabel {
		t.Fatalf("U should add the up-next label, got %v", got)
	}
	// U again removes it (toggle)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = nm.(model)
	if got := m.cache.Items["1"].Labels; len(got) != 1 || got[0] != m.settings.FollowupLabel {
		t.Fatalf("U again should remove only the up-next label, got %v", got)
	}
}

func TestTokenCheckedValidLeavesOnboard(t *testing.T) {
	m := newTestModel()
	m.cache = newCache()
	m.width, m.height = 100, 40
	m.mode = modeOnboard
	nm, cmd := m.Update(tokenCheckedMsg{valid: true})
	mm := nm.(model)
	// leaves the token prompt — either to the list or the first-run label step
	if mm.mode == modeOnboard {
		t.Fatal("valid token should leave the token onboarding screen")
	}
	if !mm.online {
		t.Fatal("valid token should mark online")
	}
	if cmd == nil {
		t.Fatal("valid token should trigger a sync")
	}
}

func TestTokenCheckedAuthErrStaysOnboard(t *testing.T) {
	m := newTestModel()
	m.cache = newCache()
	m.width, m.height = 100, 40
	m.mode = modeList
	nm, _ := m.Update(tokenCheckedMsg{authErr: true})
	if nm.(model).mode != modeOnboard {
		t.Fatal("a rejected token should drop into onboarding")
	}
}

func TestHelpPageToggle(t *testing.T) {
	m := newTestModel()
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
	m := newTestModel()
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
	m := newTestModel()
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
	// open the due-date picker
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if m.mode != modeDuePick {
		t.Fatal("'t' should open the due-date picker")
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
	m := newTestModel()
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
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Priority: "p1", Content: "urgent"},
		{ID: "2", Priority: "p4", Content: "normal"},
		{ID: "3", Priority: "p1", Content: "urgent 2"},
	}})
	m = nm.(model)
	// open priority picker (now on '!')
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
	m := newTestModel()
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
	first := firstTask(m)
	if first.Priority != "p1" {
		t.Fatalf("after sort by priority, first should be p1, got %s", first.Priority)
	}
	// reverse
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = nm.(model)
	if !m.sortDesc {
		t.Fatal("pressing 1 again should reverse the sort")
	}
	if firstTask(m).Priority != "p4" {
		t.Fatal("reversed priority sort should put p4 first")
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: sampleTasks()})
	m = nm.(model)
	// press x -> confirm mode
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = nm.(model)
	if m.mode != modeConfirm {
		t.Fatal("x should open the delete confirm")
	}
	// press n -> cancel back to list
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nm.(model).mode != modeList {
		t.Fatal("n should cancel delete")
	}
}

func TestDeadlineAndTodayFilters(t *testing.T) {
	today := todayStr()
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: []Task{
		{ID: "1", Content: "due today", DueDate: today},
		{ID: "2", Content: "overdue", DueDate: "2020-01-01"},
		{ID: "3", Content: "deadline today", Deadline: today},
		{ID: "4", Content: "deadline past", Deadline: "2020-01-01"},
		{ID: "5", Content: "no dates"},
	}})
	m = nm.(model)
	// t → due today only (task 1)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("t should show 1 due-today task, got %d", got)
	}
	// home, then T → due today or overdue (tasks 1,2)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m = nm.(model)
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("T should show 2 due/overdue tasks, got %d", got)
	}
	// d → deadline today (task 3)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")}) // home first
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = nm.(model)
	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("d should show 1 deadline-today task, got %d", got)
	}
	// D → deadline today or earlier (tasks 3,4)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	m = nm.(model)
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("D should show 2 deadline-due tasks, got %d", got)
	}
}

func TestVersionShownInHeader(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 120, 40
	m.list.SetSize(120, 36)
	if !strings.Contains(m.View(), "todo-ui "+version) {
		t.Fatal("header should show the todo-ui version")
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	nm, _ := m.Update(tasksLoadedMsg{tasks: sampleTasks()})
	out := nm.(model).View()
	if out == "" {
		t.Fatal("view should render content")
	}
}

func TestArchiveRemovesFromRecents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Japan June 2026"}
	m.deriveAll()
	p := Project{ID: "p", Name: "#Japan June 2026"}
	m.recents = []Project{p}

	m.archiveProjectLocal(p)
	for _, r := range m.recents {
		if r.ID == "p" {
			t.Fatal("an archived project must be removed from the recents list")
		}
	}

	// Delete should also prune recents.
	m.cache.Projects["q"] = apiProject{ID: "q", Name: "Old"}
	m.deriveAll()
	q := Project{ID: "q", Name: "#Old"}
	m.recents = []Project{q}
	m.deleteProjectLocal(q)
	for _, r := range m.recents {
		if r.ID == "q" {
			t.Fatal("a deleted project must be removed from the recents list")
		}
	}
}

func TestCompletedFetchMergesServerItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 100, 30
	m.list.SetSize(100, 24)
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Work"}
	m.deriveAll()
	m.projectView = "#Work"
	m.completedView = 2 // completed-only
	m.applyView()

	// Simulate the server returning a completed task not in the local cache.
	nm, _ := m.Update(completedFetchedMsg{items: []apiItem{
		{ID: "srv1", Content: "From server", ProjectID: "p", Checked: true, AddedAt: "2026-01-01"},
	}})
	m = nm.(model)

	if _, ok := m.cache.Items["srv1"]; !ok {
		t.Fatal("server completed item should be merged into the cache")
	}
	found := false
	for _, it := range m.list.Items() {
		ti := it.(taskItem)
		if !ti.sep && ti.t.Content == "From server" && ti.t.Done {
			found = true
		}
	}
	if !found {
		t.Fatal("server completed task should appear in the completed view")
	}
}

func TestCompletedViewToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 100, 30
	m.list.SetSize(100, 24)
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Work"}
	m.cache.Items["a"] = apiItem{ID: "a", Content: "Active one", ProjectID: "p", AddedAt: "2026-01-02"}
	m.cache.Items["b"] = apiItem{ID: "b", Content: "Done one", ProjectID: "p", Checked: true, AddedAt: "2026-01-01"}
	m.deriveAll()
	m.projectView = "#Work" // completed view is per-project
	m.applyView()

	countItems := func() (active, done, seps int) {
		for _, it := range m.list.Items() {
			ti := it.(taskItem)
			switch {
			case ti.sep:
				seps++
			case ti.t.Done:
				done++
			default:
				active++
			}
		}
		return
	}

	// Default: active only.
	if a, d, _ := countItems(); a != 1 || d != 0 {
		t.Fatalf("default view should show 1 active, 0 done; got %d/%d", a, d)
	}
	// Y → active + completed (with a separator).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m = nm.(model)
	if a, d, s := countItems(); a != 1 || d != 1 || s != 1 {
		t.Fatalf("both view should show 1 active, 1 done, 1 sep; got %d/%d/%d", a, d, s)
	}
	// Y → completed only.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m = nm.(model)
	if a, d, s := countItems(); a != 0 || d != 1 || s != 0 {
		t.Fatalf("completed-only view should show 0 active, 1 done; got %d/%d/%d", a, d, s)
	}
	// Completing a completed task is blocked (read-only).
	m.list.Select(0)
	q := len(m.queue)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = nm.(model)
	if len(m.queue) != q {
		t.Fatal("completing a read-only completed task should be blocked")
	}
	// Capital C reopens the highlighted completed task.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	m = nm.(model)
	if m.cache.Items["b"].Checked {
		t.Fatal("C should reopen (un-complete) the highlighted task")
	}
	reopened := false
	for _, c := range m.queue {
		if c.Type == "item_uncomplete" {
			reopened = true
		}
	}
	if !reopened {
		t.Fatal("C should queue an item_uncomplete command")
	}
	// Y → back to active only.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m = nm.(model)
	if m.completedView != 0 {
		t.Fatalf("Y should cycle back to active-only, got %d", m.completedView)
	}
}

func TestCompletedSeparatorSkippedOnNav(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.width, m.height = 100, 30
	m.list.SetSize(100, 24)
	m.cache.Projects["p"] = apiProject{ID: "p", Name: "Work"}
	m.cache.Items["a"] = apiItem{ID: "a", Content: "Active", ProjectID: "p", AddedAt: "2026-01-02"}
	m.cache.Items["b"] = apiItem{ID: "b", Content: "Done", ProjectID: "p", Checked: true, AddedAt: "2026-01-01"}
	m.deriveAll()
	m.projectView = "#Work"
	m.applyView()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")}) // active+completed
	m = nm.(model)

	m.list.Select(0) // the single active task
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	it, ok := m.list.SelectedItem().(taskItem)
	if !ok || it.sep {
		t.Fatalf("down should skip the separator, landed on sep=%v", ok && it.sep)
	}
	if !it.t.Done {
		t.Fatalf("down from the last active task should land on the first completed task, got %q", it.t.Content)
	}
}

func TestColonQuit(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	// ":" opens the command line.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = nm.(model)
	if m.mode != modeCommand {
		t.Fatalf(": should open the command line, mode=%d", m.mode)
	}
	for _, r := range "q" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	// Enter runs :q → should issue tea.Quit.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(":q then Enter should quit (expected a Quit command)")
	}
	if msg := cmd(); msg == nil {
		t.Fatal(":q should produce a quit message")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf(":q should produce tea.QuitMsg, got %T", msg)
	}
}
