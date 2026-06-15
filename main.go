package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// ---------- styling ----------

var (
	brandRed = lipgloss.Color("#E44332")
	dimColor = lipgloss.Color("#6C6C6C")
	subColor = lipgloss.Color("#9A9A9A")

	prioColors = map[string]lipgloss.Color{
		"p1": lipgloss.Color("#E44332"),
		"p2": lipgloss.Color("#EB8909"),
		"p3": lipgloss.Color("#246FE0"),
		"p4": lipgloss.Color("#808080"),
	}

	titleBarStyle = lipgloss.NewStyle().
			Background(brandRed).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().Foreground(subColor).Padding(0, 1)
	errStyle    = lipgloss.NewStyle().Foreground(brandRed).Bold(true).Padding(0, 1)
	helpStyle   = lipgloss.NewStyle().Foreground(dimColor).Padding(0, 1)

	promptBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(brandRed).
			Padding(0, 1)
)

// ---------- list item ----------

type taskItem struct{ t Task }

func (i taskItem) FilterValue() string {
	return i.t.Content + " " + i.t.Project + " " + i.t.Labels
}

// taskDelegate renders each task across two lines with a priority-coloured marker.
type taskDelegate struct{}

func (d taskDelegate) Height() int                             { return 2 }
func (d taskDelegate) Spacing() int                            { return 1 }
func (d taskDelegate) Update(tea.Msg, *list.Model) tea.Cmd     { return nil }
func (d taskDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(taskItem)
	if !ok {
		return
	}
	t := it.t
	selected := index == m.Index()

	pc := prioColors[t.Priority]
	if pc == "" {
		pc = prioColors["p4"]
	}

	marker := "  "
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DDDDDD"))
	if selected {
		marker = lipgloss.NewStyle().Foreground(pc).Bold(true).Render("▌ ")
		titleStyle = titleStyle.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}

	prio := lipgloss.NewStyle().Foreground(pc).Bold(true).Render(t.Priority)

	// meta line: #project · due · @labels
	var meta []string
	if t.Project != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8")).Render(t.Project))
	}
	if strings.TrimSpace(t.DueDate) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Render(t.DueDate))
	}
	if strings.TrimSpace(t.Labels) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379")).Render(t.Labels))
	}
	metaLine := lipgloss.NewStyle().Foreground(subColor).Render(strings.Join(meta, "  ·  "))

	line1 := fmt.Sprintf("%s%s  %s", marker, prio, titleStyle.Render(t.Content))
	line2 := "    " + metaLine
	fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// ---------- project picker item ----------

type projItem struct{ p Project }

func (i projItem) FilterValue() string { return i.p.Name }

type projDelegate struct{}

func (d projDelegate) Height() int                         { return 1 }
func (d projDelegate) Spacing() int                        { return 0 }
func (d projDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d projDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(projItem)
	if !ok {
		return
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8"))
	cur := "  "
	if index == m.Index() {
		cur = lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("▸ ")
		style = style.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}
	fmt.Fprintf(w, "%s%s", cur, style.Render(it.p.Name))
}

// ---------- model ----------

type mode int

const (
	modeList mode = iota
	modeAdd
	modeSearch
	modeConfirm
	modeProjectPick
	modeHelp
	modeDetail       // viewing a single task
	modeDetailEdit   // editing one field of the task in detail view
	modePriorityPick // choosing a priority to filter by
)

// editField is which task field the detail editor is changing.
type editField int

const (
	efDate editField = iota
	efLabels
	efContent
)

// viewState captures everything that defines what the task list shows, so we
// can push/pop it for browser-style back navigation.
type viewState struct {
	filter       string
	textQuery    string
	projectView  string
	priorityView string // "" = any, else "p1".."p4"
}

// sortMode is how the task list is ordered.
type sortMode int

const (
	sortNone     sortMode = iota // original Todoist order
	sortPriority                 // p1 → p4
	sortDue                      // soonest due first (no date last)
	sortProject                  // project name A→Z
	sortName                     // task content A→Z
	sortLabels                   // labels A→Z
)

func (s sortMode) label() string {
	switch s {
	case sortPriority:
		return "priority"
	case sortDue:
		return "due date"
	case sortProject:
		return "project"
	case sortName:
		return "name"
	case sortLabels:
		return "labels"
	default:
		return "default"
	}
}

// pickIntent distinguishes why the project picker is open.
type pickIntent int

const (
	pickAdd  pickIntent = iota // choosing a project to add a task into
	pickView                   // choosing a project to filter the view by
)

type model struct {
	list        list.Model
	projList    list.Model
	input       textinput.Model
	mode        mode
	pickIntent  pickIntent // what the project picker is for (add vs view)
	projects     []Project  // all projects (source for the picker)
	allTasks     []Task     // full set from the last server load
	filter       string     // active server-side Todoist filter (working value)
	textQuery    string     // local case-insensitive text search (working value)
	projectView  string     // local project filter, display name e.g. "#Bills" (working value)
	priorityView string     // local priority filter "p1".."p4" (working value)
	prioCursor   int        // cursor in the priority picker
	loadedFilter string     // the server filter that allTasks currently reflects
	current      viewState  // the committed view currently shown
	history      []viewState // back-stack of previously committed views
	sortMode     sortMode   // current ordering of the task list
	sortDesc     bool       // reverse the current ordering
	detailTask   Task       // task shown in the detail view
	detailID     string     // id of the task in the detail view
	editField    editField  // which field the detail editor is editing
	helpOffset   int        // scroll offset of the help page
	addProject  Project    // project chosen for the task currently being added
	lastProject Project // most recently used project (remembered across runs)
	status      string
	err         string
	width       int
	height      int
	loading     bool
}

// messages
type tasksLoadedMsg struct {
	tasks  []Task
	filter string
}
type projectsLoadedMsg struct{ projects []Project }
type errMsg struct{ err error }
type actionMsg struct{ status string }

func initialModel() model {
	l := list.New(nil, taskDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true) // local fuzzy filter via list's own "/"... we use our own instead
	l.SetShowFilter(false)
	l.DisableQuitKeybindings()

	pl := list.New(nil, projDelegate{}, 0, 0)
	pl.SetShowTitle(false)
	pl.SetShowStatusBar(false)
	pl.SetShowHelp(false)
	pl.SetFilteringEnabled(true)
	pl.SetShowFilter(true)
	pl.DisableQuitKeybindings()

	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 200

	return model{
		list:        l,
		projList:    pl,
		input:       ti,
		mode:        modeList,
		lastProject: LoadLastProject(),
		loading:     true,
		status:      "loading…",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadTasks(""), loadProjects())
}

// commands
func loadTasks(filter string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := ListTasks(filter)
		if err != nil {
			return errMsg{err}
		}
		return tasksLoadedMsg{tasks: tasks, filter: filter}
	}
}

// (project-aware add lives in quickAddInProject below)

func loadProjects() tea.Cmd {
	return func() tea.Msg {
		ps, err := ListProjects()
		if err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg{ps}
	}
}

func isInbox(p Project) bool {
	return p.ID == "" || strings.EqualFold(strings.TrimPrefix(p.Name, "#"), "Inbox")
}

// quickAddInProject creates a task via natural-language quick-add (so dates,
// priority and labels in the text are parsed), then moves it into the chosen
// project. Quick-add can't reliably route multi-word project names, so we
// diff the task list before/after and re-home the newly created task(s).
func quickAddInProject(text string, proj Project) tea.Cmd {
	return func() tea.Msg {
		var before map[string]bool
		if !isInbox(proj) {
			before = map[string]bool{}
			if prev, err := ListTasks(""); err == nil {
				for _, t := range prev {
					before[t.ID] = true
				}
			}
		}
		if err := QuickAdd(text); err != nil {
			return errMsg{err}
		}
		if !isInbox(proj) {
			if after, err := ListTasks(""); err == nil {
				for _, t := range after {
					if !before[t.ID] {
						_ = ModifyProject(t.ID, proj.ID, prioNum(t.Priority))
					}
				}
			}
		}
		return actionMsg{status: "added to " + proj.Name + ": " + text}
	}
}

func setPriorityCmd(id string, p int) tea.Cmd {
	return func() tea.Msg {
		if err := SetPriority(id, p); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: fmt.Sprintf("priority → p%d", p)}
	}
}

func setDateCmd(id, date string, prio int) tea.Cmd {
	return func() tea.Msg {
		if err := SetDate(id, date, prio); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: "due date updated"}
	}
}

func setLabelsCmd(id, labels string, prio int) tea.Cmd {
	return func() tea.Msg {
		if err := SetLabels(id, labels, prio); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: "labels updated"}
	}
}

func setContentCmd(id, content string, prio int) tea.Cmd {
	return func() tea.Msg {
		if err := SetContent(id, content, prio); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: "name updated"}
	}
}

func closeCmd(id, content string) tea.Cmd {
	return func() tea.Msg {
		if err := CloseTask(id); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: "completed: " + content}
	}
}

func deleteCmd(id, content string) tea.Cmd {
	return func() tea.Msg {
		if err := DeleteTask(id); err != nil {
			return errMsg{err}
		}
		return actionMsg{status: "deleted: " + content}
	}
}

func (m *model) selectedTask() (Task, bool) {
	it, ok := m.list.SelectedItem().(taskItem)
	if !ok {
		return Task{}, false
	}
	return it.t, true
}

// applyView rebuilds the visible list from allTasks, narrowed by the local
// text query (case-insensitive substring over content, project and labels).
func (m *model) applyView() {
	q := strings.ToLower(strings.TrimSpace(m.textQuery))
	var matched []Task
	for _, t := range m.allTasks {
		if m.projectView != "" && t.Project != m.projectView {
			continue
		}
		if m.priorityView != "" && t.Priority != m.priorityView {
			continue
		}
		if q != "" {
			hay := strings.ToLower(t.Content + " " + t.Project + " " + t.Labels)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		matched = append(matched, t)
	}
	m.sortTasks(matched)
	items := make([]list.Item, len(matched))
	for i, t := range matched {
		items[i] = taskItem{t}
	}
	m.list.SetItems(items)
}

// dueSortKey turns a CLI due string ("25/06/05(Thu) 09:00") into a
// lexicographically comparable key; empty dates sort last.
func dueSortKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "99999999 9999"
	}
	date := s
	if i := strings.Index(s, "("); i >= 0 {
		date = s[:i]
	}
	date = strings.ReplaceAll(strings.TrimSpace(date), "/", "")
	tm := "0000"
	if i := strings.Index(s, ")"); i >= 0 {
		if rest := strings.ReplaceAll(strings.TrimSpace(s[i+1:]), ":", ""); rest != "" {
			tm = rest
		}
	}
	return date + " " + tm
}

// sortTasks orders ts in place according to the current sort mode/direction.
func (m *model) sortTasks(ts []Task) {
	if m.sortMode == sortNone {
		return
	}
	var less func(i, j int) bool
	switch m.sortMode {
	case sortPriority:
		less = func(i, j int) bool { return ts[i].Priority < ts[j].Priority }
	case sortDue:
		less = func(i, j int) bool { return dueSortKey(ts[i].DueDate) < dueSortKey(ts[j].DueDate) }
	case sortProject:
		less = func(i, j int) bool {
			return strings.ToLower(ts[i].Project) < strings.ToLower(ts[j].Project)
		}
	case sortName:
		less = func(i, j int) bool {
			return strings.ToLower(ts[i].Content) < strings.ToLower(ts[j].Content)
		}
	case sortLabels:
		less = func(i, j int) bool {
			return strings.ToLower(ts[i].Labels) < strings.ToLower(ts[j].Labels)
		}
	}
	sort.SliceStable(ts, func(i, j int) bool {
		if m.sortDesc {
			return less(j, i)
		}
		return less(i, j)
	})
}

// scopeStatus describes the current view and its visible count.
func (m *model) scopeStatus() string {
	n := len(m.list.Items())
	if m.filter != "" {
		return fmt.Sprintf("filter: %s — %d", m.filter, n)
	}
	var parts []string
	if m.projectView != "" {
		parts = append(parts, m.projectView)
	}
	if m.priorityView != "" {
		parts = append(parts, m.priorityView)
	}
	if m.textQuery != "" {
		parts = append(parts, "“"+m.textQuery+"”")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d tasks", n)
	}
	return fmt.Sprintf("%s — %d", strings.Join(parts, " · "), n)
}

// applyState sets the working view variables and refreshes the list, reloading
// from the server only when the needed server filter differs from what's loaded.
func (m *model) applyState(s viewState) tea.Cmd {
	m.filter, m.textQuery, m.projectView, m.priorityView = s.filter, s.textQuery, s.projectView, s.priorityView
	if s.filter != m.loadedFilter {
		m.loading = true
		return loadTasks(s.filter)
	}
	m.applyView()
	m.status = m.scopeStatus()
	return nil
}

// commit navigates to a new view, pushing the current one onto the back-stack.
func (m *model) commit(s viewState) tea.Cmd {
	if s == m.current {
		return m.applyState(s) // no-op navigation, just refresh
	}
	m.history = append(m.history, m.current)
	m.current = s
	return m.applyState(s)
}

// goBack pops the back-stack and restores the previous view.
func (m *model) goBack() tea.Cmd {
	if len(m.history) == 0 {
		m.status = "nothing to go back to"
		return nil
	}
	s := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.current = s
	return m.applyState(s)
}

// isFilterExpr reports whether a search string looks like a Todoist filter
// expression (operators or known keywords) rather than plain search text.
func isFilterExpr(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "|&!#@():") {
		return true
	}
	low := strings.ToLower(s)
	keywords := []string{
		"today", "tomorrow", "yesterday", "overdue", "recurring",
		"no date", "no time", "no label", "no priority", "no due",
		"due", "date", "before", "after", "days", "weeks", "week",
		"assigned", "shared", "subtask", "p1", "p2", "p3", "p4",
	}
	for _, k := range keywords {
		if low == k || strings.HasPrefix(low, k+" ") || strings.Contains(low, " "+k+" ") || strings.HasSuffix(low, " "+k) {
			return true
		}
	}
	return false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.projList.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.setPickerItems()
		return m, nil

	case tasksLoadedMsg:
		m.loading = false
		m.filter = msg.filter
		m.loadedFilter = msg.filter
		m.allTasks = msg.tasks
		m.applyView()
		m.status = m.scopeStatus()
		// Keep the detail view in sync after an edit/reload.
		if (m.mode == modeDetail || m.mode == modeDetailEdit) && m.detailID != "" {
			found := false
			for _, t := range m.allTasks {
				if t.ID == m.detailID {
					m.detailTask = t
					found = true
					break
				}
			}
			if !found {
				// task no longer present (e.g. completed) → return to list
				m.mode = modeList
			}
		}
		return m, nil

	case actionMsg:
		m.status = msg.status
		m.loading = true
		return m, loadTasks(m.filter) // reload current view

	case errMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeAdd, modeSearch:
			return m.updateInput(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modeProjectPick:
			return m.updateProjectPick(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeDetailEdit:
			return m.updateDetailEdit(msg)
		case modePriorityPick:
			return m.updatePriorityPick(msg)
		}
	}

	// default: pass to list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "a":
		// Open the project picker, pre-selecting the last-used project.
		m.mode = modeProjectPick
		m.pickIntent = pickAdd
		m.err = ""
		m.setPickerItems()
		m.selectLastProject()
		return m, nil
	case "p":
		// View-by-project: prompt with the project list, then filter to it.
		m.mode = modeProjectPick
		m.pickIntent = pickView
		m.err = ""
		m.setPickerItems()
		m.selectLastProject()
		return m, nil
	case "A":
		// Fast path: add straight to the last-used project, skipping the picker.
		if m.lastProject.ID == "" {
			m.mode = modeProjectPick
			m.pickIntent = pickAdd
			m.setPickerItems()
			m.selectLastProject()
			return m, nil
		}
		m.addProject = m.lastProject
		m.mode = modeAdd
		m.err = ""
		m.input.Placeholder = "Buy milk @errand tomorrow 9am p1"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "/":
		m.mode = modeSearch
		m.err = ""
		m.input.Placeholder = "type words to search · or a Todoist filter (today, #Personal, @label)"
		prefill := m.textQuery
		if m.filter != "" {
			prefill = m.filter
		}
		m.input.SetValue(prefill)
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "enter":
		if t, ok := m.selectedTask(); ok {
			m.detailTask = t
			m.detailID = t.ID
			m.mode = modeDetail
			m.err = ""
		}
		return m, nil
	case "c":
		if t, ok := m.selectedTask(); ok {
			return m, closeCmd(t.ID, t.Content)
		}
	case "o":
		// Ongoing — show all tasks tagged @ongoing.
		return m, m.commit(viewState{filter: "@ongoing"})
	case "P":
		// Filter by priority — open the priority picker.
		m.mode = modePriorityPick
		m.err = ""
		m.prioCursor = 0
		for i, o := range priorityOptions {
			if o.val == m.priorityView {
				m.prioCursor = i
			}
		}
		return m, nil
	case "d":
		if _, ok := m.selectedTask(); ok {
			m.mode = modeConfirm
			return m, nil
		}
	case "r":
		m.loading = true
		m.status = "refreshing…"
		return m, loadTasks(m.filter)
	case "s":
		// Sync the underlying todoist client, then refresh.
		m.loading = true
		m.status = "syncing…"
		return m, tea.Sequence(syncCmd(), loadTasks(m.filter))
	case "b":
		return m, m.goBack()
	case "h", "esc":
		// Home — back to the all-projects / all-tasks view (undoable with b).
		return m, m.commit(viewState{})
	case "H", "?":
		m.mode = modeHelp
		m.helpOffset = 0
		return m, nil
	case "1":
		m.setSort(sortPriority)
		return m, nil
	case "2":
		m.setSort(sortDue)
		return m, nil
	case "3":
		m.setSort(sortProject)
		return m, nil
	case "4":
		m.setSort(sortName)
		return m, nil
	case "5":
		m.setSort(sortLabels)
		return m, nil
	case "0":
		m.setSort(sortNone)
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// setSort applies a sort mode; pressing the same mode again flips direction.
func (m *model) setSort(s sortMode) {
	if s != sortNone && m.sortMode == s {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortMode = s
		m.sortDesc = false
	}
	m.applyView()
	if s == sortNone {
		m.status = "sort: default order"
		return
	}
	dir := "↑"
	if m.sortDesc {
		dir = "↓"
	}
	m.status = fmt.Sprintf("sort: %s %s — %d", s.label(), dir, len(m.list.Items()))
}

func syncCmd() tea.Cmd {
	return func() tea.Msg {
		_ = Sync()
		return nil
	}
}

// allProjectsID is the sentinel for the "All Projects" picker entry (view mode).
const allProjectsID = "__all__"

// setPickerItems rebuilds the project picker list. In view mode it prepends an
// "All Projects" entry so you can clear the project filter from the menu.
func (m *model) setPickerItems() {
	var items []list.Item
	if m.pickIntent == pickView {
		items = append(items, projItem{Project{ID: allProjectsID, Name: "↩ All Projects"}})
	}
	for _, p := range m.projects {
		items = append(items, projItem{p})
	}
	m.projList.SetItems(items)
}

// selectLastProject moves the picker cursor onto the remembered project.
func (m *model) selectLastProject() {
	if m.lastProject.ID == "" {
		return
	}
	for i, it := range m.projList.Items() {
		if p, ok := it.(projItem); ok && p.p.ID == m.lastProject.ID {
			m.projList.Select(i)
			return
		}
	}
}

// priorityOptions lists the choices in the priority picker (P).
var priorityOptions = []struct {
	val   string // "" = any, else p1..p4
	label string
}{
	{"", "↩ All priorities"},
	{"p1", "p1 — Urgent"},
	{"p2", "p2 — High"},
	{"p3", "p3 — Medium"},
	{"p4", "p4 — Normal / none"},
}

// pickPriority commits a priority filter, keeping the current project & search.
func (m model) pickPriority(val string) (tea.Model, tea.Cmd) {
	m.mode = modeList
	cmd := m.commit(viewState{
		projectView:  m.current.projectView,
		textQuery:    m.current.textQuery,
		priorityView: val,
	})
	return m, cmd
}

func (m model) updatePriorityPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "b", "q":
		m.mode = modeList
		return m, nil
	case "up", "k":
		if m.prioCursor > 0 {
			m.prioCursor--
		}
		return m, nil
	case "down", "j":
		if m.prioCursor < len(priorityOptions)-1 {
			m.prioCursor++
		}
		return m, nil
	case "enter":
		return m.pickPriority(priorityOptions[m.prioCursor].val)
	case "0", "a":
		return m.pickPriority("")
	case "1", "2", "3", "4":
		return m.pickPriority("p" + msg.String())
	}
	return m, nil
}

func (m model) updateProjectPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While filtering, let the list consume keys (incl. enter/esc).
	if m.projList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.projList, cmd = m.projList.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		it, ok := m.projList.SelectedItem().(projItem)
		if !ok {
			m.mode = modeList
			return m, nil
		}
		if m.pickIntent == pickView {
			m.mode = modeList
			if it.p.ID == allProjectsID {
				return m, m.commit(viewState{}) // back to all projects
			}
			return m, m.commit(viewState{projectView: it.p.Name})
		}
		// pickAdd: remember the project and move to the add input.
		m.addProject = it.p
		m.lastProject = it.p
		SaveLastProject(it.p)
		m.mode = modeAdd
		m.input.Placeholder = "Buy milk @errand tomorrow 9am p1"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.projList, cmd = m.projList.Update(msg)
	return m, cmd
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		// restore the committed view (cancels any live search-preview narrowing)
		m.textQuery = m.current.textQuery
		m.projectView = m.current.projectView
		m.filter = m.current.filter
		m.applyView()
		m.status = m.scopeStatus()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		switch m.mode {
		case modeAdd:
			m.mode = modeList
			m.input.Blur()
			if val == "" {
				return m, nil
			}
			m.loading = true
			return m, quickAddInProject(val, m.addProject)
		case modeSearch:
			m.mode = modeList
			m.input.Blur()
			if isFilterExpr(val) {
				// power query → server-side Todoist filter
				return m, m.commit(viewState{filter: val})
			}
			// plain words → local text search, kept within any active project view
			return m, m.commit(viewState{projectView: m.current.projectView, textQuery: val})
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	// Live local preview while typing a plain-text search.
	if m.mode == modeSearch {
		cur := m.input.Value()
		if !isFilterExpr(cur) {
			m.textQuery = cur
			m.applyView()
		}
	}
	return m, cmd
}

// startEdit opens the field editor in the detail view, prefilled appropriately.
func (m *model) startEdit(f editField, placeholder, prefill string) tea.Cmd {
	m.editField = f
	m.mode = modeDetailEdit
	m.input.Placeholder = placeholder
	m.input.SetValue(prefill)
	m.input.CursorEnd()
	m.input.Focus()
	return textinput.Blink
}

// labelsCSV converts the CSV "@a @b" / "@a,@b" label string to "a,b" for editing.
func labelsCSV(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	for i, f := range fields {
		fields[i] = strings.TrimPrefix(strings.TrimSpace(f), "@")
	}
	return strings.Join(fields, ",")
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "b", "q", "enter":
		m.mode = modeList
		return m, nil
	case "c":
		m.mode = modeList
		m.loading = true
		return m, closeCmd(m.detailID, m.detailTask.Content)
	case "1", "2", "3", "4":
		p := int(msg.String()[0] - '0')
		m.loading = true
		return m, setPriorityCmd(m.detailID, p)
	case "t", "D":
		return m, m.startEdit(efDate, "today · tomorrow 9am · 2026/07/01 · (empty cancels)", "")
	case "l":
		return m, m.startEdit(efLabels, "comma-separated, e.g. ongoing,follow-up", labelsCSV(m.detailTask.Labels))
	case "e":
		return m, m.startEdit(efContent, "task name", m.detailTask.Content)
	case "r":
		m.loading = true
		return m, loadTasks(m.filter)
	}
	return m, nil
}

func (m model) updateDetailEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeDetail
		m.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		m.mode = modeDetail
		m.input.Blur()
		if val == "" && m.editField != efLabels {
			return m, nil // empty cancels (except labels, where empty clears them)
		}
		m.loading = true
		prio := prioNum(m.detailTask.Priority)
		switch m.editField {
		case efDate:
			return m, setDateCmd(m.detailID, val, prio)
		case efLabels:
			return m, setLabelsCmd(m.detailID, val, prio)
		case efContent:
			return m, setContentCmd(m.detailID, val, prio)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = modeList
		if t, ok := m.selectedTask(); ok {
			m.loading = true
			return m, deleteCmd(t.ID, t.Content)
		}
	default:
		m.mode = modeList
	}
	return m, nil
}

// ---------- view ----------

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	title := titleBarStyle.Render("✓ Todoist")
	scope := "  all tasks"
	if m.filter != "" {
		scope = "  filter: " + m.filter
	} else {
		var parts []string
		if m.projectView != "" {
			parts = append(parts, "project: "+m.projectView)
		}
		if m.priorityView != "" {
			parts = append(parts, "priority: "+m.priorityView)
		}
		if m.textQuery != "" {
			parts = append(parts, "search: "+m.textQuery)
		}
		if len(parts) > 0 {
			scope = "  " + strings.Join(parts, " · ")
		}
	}
	if m.sortMode != sortNone {
		dir := "↑"
		if m.sortDesc {
			dir = "↓"
		}
		scope += lipgloss.NewStyle().Foreground(dimColor).Render(fmt.Sprintf("   ⇅ %s %s", m.sortMode.label(), dir))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, statusStyle.Render(scope))

	if m.mode == modeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.helpView())
	}

	if m.mode == modeDetail || m.mode == modeDetailEdit {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.detailView())
	}

	if m.mode == modePriorityPick {
		hint := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("Filter by priority")
		var rows []string
		rows = append(rows, "", "  "+hint, "")
		for i, o := range priorityOptions {
			pc := prioColors[o.val]
			if pc == "" {
				pc = lipgloss.Color("#DDDDDD")
			}
			cur := "   "
			st := lipgloss.NewStyle().Foreground(pc)
			if i == m.prioCursor {
				cur = lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(" ▸ ")
				st = st.Bold(true)
			}
			rows = append(rows, cur+st.Render(o.label))
		}
		rows = append(rows, "", helpStyle.Render("  ↑/↓ move · 1-4 priority · 0 all · enter select · esc cancel"))
		return lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	var body string
	switch m.mode {
	case modeProjectPick:
		prompt := "Add to which project?"
		if m.pickIntent == pickView {
			prompt = "View which project?"
		}
		hint := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(prompt)
		help := helpStyle.Render("type to filter · enter select · esc cancel")
		picker := lipgloss.JoinVertical(lipgloss.Left, hint, m.projList.View(), help)
		return lipgloss.JoinVertical(lipgloss.Left, header, picker)
	case modeAdd:
		proj := m.addProject.Name
		if proj == "" {
			proj = "#Inbox"
		}
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).
			Render("Add → " + proj + "  ")
		body = promptBox.Width(m.width - 4).Render(label + m.input.View())
	case modeSearch:
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("Search    ")
		body = promptBox.Width(m.width - 4).Render(label + m.input.View())
	case modeConfirm:
		t, _ := m.selectedTask()
		q := lipgloss.NewStyle().Foreground(brandRed).Bold(true).
			Render(fmt.Sprintf("Delete \"%s\"?  (y/n)", t.Content))
		body = promptBox.Width(m.width - 4).Render(q)
	}

	listView := m.list.View()

	footer := m.footer()

	if body != "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, body, listView, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, listView, footer)
}

// detailView renders the single-task detail / edit screen.
func (m model) detailView() string {
	t := m.detailTask
	label := lipgloss.NewStyle().Foreground(subColor)
	key := lipgloss.NewStyle().Foreground(brandRed).Bold(true)

	pc := prioColors[t.Priority]
	if pc == "" {
		pc = prioColors["p4"]
	}
	field := func(name, val string, valStyle lipgloss.Style) string {
		if strings.TrimSpace(val) == "" {
			val = "—"
		}
		return "  " + label.Render(fmt.Sprintf("%-10s", name)) + valStyle.Render(val)
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(t.Content)

	lines := []string{
		"",
		"  " + title,
		"",
		field("Priority", t.Priority, lipgloss.NewStyle().Foreground(pc).Bold(true)),
		field("Due", t.DueDate, lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B"))),
		field("Project", t.Project, lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8"))),
		field("Labels", t.Labels, lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379"))),
		field("ID", t.ID, label),
		"",
	}

	if m.mode == modeDetailEdit {
		titles := map[editField]string{efDate: "Set due date", efLabels: "Set labels (comma-separated)", efContent: "Edit name"}
		prompt := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(titles[m.editField] + "  ")
		box := promptBox.Width(m.width - 4).Render(prompt + m.input.View())
		lines = append(lines, box, "", helpStyle.Render("  enter save · esc cancel"))
	} else {
		actions := "  " +
			key.Render("1-4") + label.Render(" priority   ") +
			key.Render("t") + label.Render(" date   ") +
			key.Render("l") + label.Render(" labels   ") +
			key.Render("e") + label.Render(" name   ") +
			key.Render("c") + label.Render(" complete   ") +
			key.Render("b/esc") + label.Render(" back")
		lines = append(lines, actions)
		if m.err != "" {
			lines = append(lines, "", errStyle.Render("⚠ "+m.err))
		} else if m.loading {
			lines = append(lines, "", statusStyle.Render("⟳ "+m.status))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// helpLines returns the full help content as individual lines (for scrolling).
func helpLines() []string {
	key := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)

	row := func(k, desc string) string {
		return "  " + key.Render(fmt.Sprintf("%-12s", k)) + dim.Render(desc)
	}

	return []string{
		"",
		head.Render("  Navigation"),
		row("↑/↓ j/k", "Move selection"),
		row("pgup/pgdn", "Page through the list"),
		row("b", "Back — return to the previous view (like a browser)"),
		row("h / esc", "Home — back to all tasks / all projects"),
		row("q ctrl+c", "Quit"),
		"",
		head.Render("  Tasks"),
		row("enter", "Open the task — view & edit date, priority, labels, name"),
		row("a", "Add a task — choose the project first"),
		row("A", "Add a task straight to the last-used project"),
		row("c", "Complete the selected task"),
		row("d", "Delete the selected task (asks y/n)"),
		row("r", "Refresh the list"),
		row("s", "Sync the todoist client, then refresh"),
		"",
		head.Render("  In the task view"),
		row("1-4", "Set priority (p1–p4)"),
		row("t", "Set due date    l  set labels    e  edit name"),
		row("c", "Complete    b/esc  back to the list"),
		"",
		head.Render("  Views"),
		row("p", "View by project (pick from the list; “↩ All Projects” to reset)"),
		row("P", "Filter by priority (pick p1–p4 from the menu)"),
		row("o", "Ongoing — show all tasks tagged @ongoing"),
		row("/", "Search — plain words search locally; filters run server-side"),
		"",
		head.Render("  Sort  (press the same number again to reverse)"),
		row("1", "Priority (p1 → p4)"),
		row("2", "Due date (soonest first, no-date last)"),
		row("3", "Project (A → Z)"),
		row("4", "Name (A → Z)"),
		row("5", "Labels (A → Z)"),
		row("0", "Default Todoist order"),
		"",
		head.Render("  Search tips"),
		row("plain text", "anvaya, pay globe — instant local search of name/project/labels"),
		row("filters", "today | overdue, #Personal & p1, @follow-up — Todoist syntax"),
		"",
		head.Render("  Add syntax (natural language)"),
		row("example", "Pay bill @bills-payment tomorrow 9am p1"),
		dim.Render("              dates, @labels and p1–p4 are parsed by Todoist;"),
		dim.Render("              the project comes from the picker."),
		"",
	}
}

// helpViewport is how many help lines fit on screen (excludes header + footer).
func (m model) helpViewport() int {
	v := m.height - 2
	if v < 1 {
		v = 1
	}
	return v
}

func (m model) maxHelpOffset() int {
	max := len(helpLines()) - m.helpViewport()
	if max < 0 {
		return 0
	}
	return max
}

// helpView renders the scrollable help page (opened with H or ?).
func (m model) helpView() string {
	lines := helpLines()
	vp := m.helpViewport()
	off := m.helpOffset
	if off > m.maxHelpOffset() {
		off = m.maxHelpOffset()
	}
	end := off + vp
	if end > len(lines) {
		end = len(lines)
	}
	window := lines[off:end]

	pos := "all"
	if m.maxHelpOffset() > 0 {
		if off == 0 {
			pos = "top"
		} else if off >= m.maxHelpOffset() {
			pos = "end"
		} else {
			pos = fmt.Sprintf("%d%%", off*100/m.maxHelpOffset())
		}
	}
	hint := helpStyle.Render(fmt.Sprintf("  j/k ↑/↓ scroll · %s · any other key closes", pos))

	return lipgloss.JoinVertical(lipgloss.Left, append(window, hint)...)
}

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		if m.helpOffset < m.maxHelpOffset() {
			m.helpOffset++
		}
		return m, nil
	case "k", "up":
		if m.helpOffset > 0 {
			m.helpOffset--
		}
		return m, nil
	case "pgdown", "ctrl+d", "f", " ":
		m.helpOffset += m.helpViewport() - 1
		if m.helpOffset > m.maxHelpOffset() {
			m.helpOffset = m.maxHelpOffset()
		}
		return m, nil
	case "pgup", "ctrl+u":
		m.helpOffset -= m.helpViewport() - 1
		if m.helpOffset < 0 {
			m.helpOffset = 0
		}
		return m, nil
	case "g", "home":
		m.helpOffset = 0
		return m, nil
	case "G", "end":
		m.helpOffset = m.maxHelpOffset()
		return m, nil
	}
	m.mode = modeList // any other key closes help
	return m, nil
}

func (m model) footer() string {
	if m.err != "" {
		return errStyle.Render("⚠ " + m.err)
	}
	keys := "a add · p project · P priority · o ongoing · / search · 1-5 sort · enter view · c done · d del · s sync · H help · q quit"
	st := m.status
	if m.loading {
		st = "⟳ " + st
	}
	left := statusStyle.Render(st)
	right := helpStyle.Render(keys)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("todoui", version)
			return
		case "--help", "-h", "help":
			fmt.Println("todoui — a terminal UI for Todoist (wraps the `todoist` CLI)")
			fmt.Println("Usage: todoui            start the UI")
			fmt.Println("       todoui --version  print version")
			fmt.Println("Press H inside the app for the keyboard reference.")
			return
		}
	}
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
	}
}
