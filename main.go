package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
)

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
	allTasks    []Task     // full set from the last server load
	filter      string     // active server-side Todoist filter
	textQuery   string     // local case-insensitive text search over loaded tasks
	projectView string     // local filter: show only this project (display name, e.g. "#Bills")
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
						_ = ModifyProject(t.ID, proj.ID)
					}
				}
			}
		}
		return actionMsg{status: "added to " + proj.Name + ": " + text}
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
	var items []list.Item
	for _, t := range m.allTasks {
		if m.projectView != "" && t.Project != m.projectView {
			continue
		}
		if q != "" {
			hay := strings.ToLower(t.Content + " " + t.Project + " " + t.Labels)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		items = append(items, taskItem{t})
	}
	m.list.SetItems(items)
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
		items := make([]list.Item, len(msg.projects))
		for i, p := range msg.projects {
			items[i] = projItem{p}
		}
		m.projList.SetItems(items)
		return m, nil

	case tasksLoadedMsg:
		m.loading = false
		m.filter = msg.filter
		m.allTasks = msg.tasks
		m.applyView()
		m.status = fmt.Sprintf("%d tasks", len(m.list.Items()))
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
		m.selectLastProject()
		return m, nil
	case "p":
		// View-by-project: prompt with the project list, then filter to it.
		m.mode = modeProjectPick
		m.pickIntent = pickView
		m.err = ""
		m.selectLastProject()
		return m, nil
	case "A":
		// Fast path: add straight to the last-used project, skipping the picker.
		if m.lastProject.ID == "" {
			m.mode = modeProjectPick
			m.pickIntent = pickAdd
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
	case "c", "enter":
		if t, ok := m.selectedTask(); ok {
			return m, closeCmd(t.ID, t.Content)
		}
	case "d":
		if _, ok := m.selectedTask(); ok {
			m.mode = modeConfirm
			return m, nil
		}
	case "r":
		m.loading = true
		m.status = "refreshing…"
		return m, tea.Sequence(syncCmd(), loadTasks(m.filter))
	case "esc":
		if m.filter == "" && (m.textQuery != "" || m.projectView != "") {
			// purely local narrowing — clear instantly, no reload needed
			m.textQuery = ""
			m.projectView = ""
			m.applyView()
			m.status = fmt.Sprintf("%d tasks", len(m.list.Items()))
			return m, nil
		}
		if m.filter != "" || m.textQuery != "" || m.projectView != "" {
			m.filter = ""
			m.textQuery = ""
			m.projectView = ""
			m.loading = true
			m.status = "cleared"
			return m, loadTasks("")
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func syncCmd() tea.Cmd {
	return func() tea.Msg {
		_ = Sync()
		return nil
	}
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
			// Filter the current view to the chosen project (local).
			m.mode = modeList
			m.projectView = it.p.Name
			m.applyView()
			m.status = fmt.Sprintf("%s — %d", it.p.Name, len(m.list.Items()))
			return m, nil
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
				m.filter = val
				m.textQuery = ""
				m.loading = true
				m.status = "filter: " + val
				return m, loadTasks(val)
			}
			// plain words → local text search over loaded tasks
			m.textQuery = val
			m.applyView()
			if val == "" {
				m.status = fmt.Sprintf("%d tasks", len(m.list.Items()))
			} else {
				m.status = fmt.Sprintf("search “%s” — %d", val, len(m.list.Items()))
			}
			return m, nil
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
		if m.textQuery != "" {
			parts = append(parts, "search: "+m.textQuery)
		}
		if len(parts) > 0 {
			scope = "  " + strings.Join(parts, " · ")
		}
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, statusStyle.Render(scope))

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

func (m model) footer() string {
	if m.err != "" {
		return errStyle.Render("⚠ " + m.err)
	}
	keys := "a add · A add→last · p project · / search · enter/c done · d del · r refresh · q quit"
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
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
	}
}
