package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

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
	if strings.TrimSpace(t.Deadline) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75")).Render("⚑ "+t.Deadline))
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

type projKind int

const (
	kindProject     projKind = iota // a normal project
	kindRecent                      // a recently-chosen project (shown at top)
	kindSep                         // a non-selectable separator row
	kindAllProjects                 // the "↩ All Projects" reset entry (view mode)
)

type projItem struct {
	p    Project
	kind projKind
}

func (i projItem) FilterValue() string {
	if i.kind == kindSep {
		return ""
	}
	return i.p.Name
}

// colors for the picker
var (
	projColor   = lipgloss.Color("#8AB4F8") // normal projects (blue)
	recentColor = lipgloss.Color("#E5C07B") // recent projects (gold)
)

type projDelegate struct{}

func (d projDelegate) Height() int                         { return 1 }
func (d projDelegate) Spacing() int                        { return 0 }
func (d projDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d projDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(projItem)
	if !ok {
		return
	}
	if it.kind == kindSep {
		fmt.Fprint(w, lipgloss.NewStyle().Foreground(dimColor).Render("  ───────────── all projects ─────────────"))
		return
	}

	selected := index == m.Index()
	var base lipgloss.Color
	prefix := "  "
	text := it.p.Name
	switch it.kind {
	case kindRecent:
		base = recentColor
		prefix = "★ "
	case kindAllProjects:
		base = recentColor
	default:
		base = projColor
	}
	style := lipgloss.NewStyle().Foreground(base)
	if selected {
		prefix = lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("▸ ")
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}
	fmt.Fprintf(w, "%s%s", prefix, style.Render(text))
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
	modeCommentAdd   // writing a new comment in the detail view
	modePriorityPick // choosing a priority to filter by
	modeOnboard      // first-run / invalid-token: prompt for the API token
	modeOnboardLabel // first-run: choose the "ongoing" label
	modeClearData    // confirm clearing token + cache + queue
	modeOptions      // settings page
	modeOptionsEdit  // editing one setting
	modeOnlineSearch // online Todoist filter query (-)
)

// editField is which task field the detail editor is changing.
type editField int

const (
	efDate editField = iota
	efDeadline
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
	sortDeadline                 // soonest deadline first (none last)
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
	case sortDeadline:
		return "deadline"
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
	cache        *Cache     // local offline-first snapshot
	queue        []Command  // pending Sync API commands (flushed on sync)
	syncing      bool       // a sync is in flight
	online       bool       // last sync succeeded
	current      viewState  // the committed view currently shown
	history      []viewState // back-stack of previously committed views
	sortMode     sortMode   // current ordering of the task list
	sortDesc     bool       // reverse the current ordering
	detailTask   Task       // task shown in the detail view
	detailID     string     // id of the task in the detail view
	editField    editField  // which field the detail editor is editing
	comments     []Comment  // comments for the task in the detail view
	commentErr   string     // error from the last comment fetch/post
	recentView   bool       // showing the recently-added tasks
	recentIDs    []string   // task IDs in recently-added order
	onlineView   bool       // showing online (server filter) search results
	onlineResults []Task    // results of the last online search
	onlineQuery  string     // last online query (for the header)
	searching    bool       // online search in flight
	onboardErr   string     // error shown on the onboarding screen
	checking     bool       // validating the token
	projQuery    string     // type-to-filter text in the project picker
	settings     Settings   // user preferences (ongoing label, sync interval)
	tickGen      int        // generation guard for the auto-sync ticker
	optCursor    int        // selected row on the options page
	helpOffset   int        // scroll offset of the help page
	addProject   Project    // project chosen for the task currently being added
	recents      []Project  // recently-chosen projects, most recent first (persisted)
	status       string
	err          string
	width        int
	height       int
}

// messages
type tasksLoadedMsg struct { // used by tests to seed the task list directly
	tasks  []Task
	filter string
}
type projectsLoadedMsg struct{ projects []Project } // used by tests to seed projects
type syncResultMsg struct {
	resp *syncResponse
	sent int // number of queued commands that were flushed
	err  error
}
type tokenCheckedMsg struct {
	valid   bool
	authErr bool // token rejected (vs. just offline)
}
type autoSyncTickMsg struct{ gen int }
type onlineResultMsg struct {
	query string
	items []apiItem
	err   error
}
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
	pl.SetFilteringEnabled(false) // we run our own type-to-filter
	pl.SetShowFilter(false)
	pl.DisableQuitKeybindings()

	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 200

	c := LoadCache()
	m := model{
		list:     l,
		projList: pl,
		input:    ti,
		mode:     modeList,
		cache:    c,
		queue:    LoadQueue(),
		recents:  LoadRecentProjects(),
		settings: LoadSettings(),
		status:   "ready",
	}
	m.deriveAll()
	if !HasToken() {
		m.beginOnboard("Welcome! Paste your Todoist API token to get started.")
	}
	return m
}

// beginOnboard switches to the token-entry screen.
func (m *model) beginOnboard(msg string) {
	m.mode = modeOnboard
	m.onboardErr = msg
	m.input.EchoMode = textinput.EchoPassword
	m.input.Placeholder = "Todoist API token"
	m.input.SetValue("")
	m.input.Focus()
}

// beginLabelOnboard asks which label marks "ongoing" tasks (first run).
func (m *model) beginLabelOnboard() {
	m.mode = modeOnboardLabel
	m.input.EchoMode = textinput.EchoNormal
	m.input.Placeholder = "ongoing"
	m.input.SetValue(m.settings.OngoingLabel)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m model) Init() tea.Cmd {
	if m.mode == modeOnboard {
		return textinput.Blink // wait for the token
	}
	// Validate the configured token at startup, then sync.
	return checkTokenCmd()
}

func checkTokenCmd() tea.Cmd {
	return func() tea.Msg {
		valid, authErr := ValidateToken()
		return tokenCheckedMsg{valid: valid, authErr: authErr}
	}
}

// autoSyncCmd schedules the next background sync tick (no-op if interval is 0).
func autoSyncCmd(gen, seconds int) tea.Cmd {
	if seconds <= 0 {
		return nil
	}
	return tea.Tick(time.Duration(seconds)*time.Second, func(time.Time) tea.Msg {
		return autoSyncTickMsg{gen: gen}
	})
}

// restartAutoSync invalidates outstanding ticks and starts a fresh one.
func (m *model) restartAutoSync() tea.Cmd {
	m.tickGen++
	return autoSyncCmd(m.tickGen, m.settings.SyncSeconds)
}

// syncNowCmd flushes the queue and pulls updates in the background.
func syncNowCmd(syncToken string, queue []Command) tea.Cmd {
	sent := len(queue)
	return func() tea.Msg {
		resp, err := DoSync(syncToken, queue)
		return syncResultMsg{resp: resp, sent: sent, err: err}
	}
}

func onlineSearchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		items, err := FilterTasks(query)
		return onlineResultMsg{query: query, items: items, err: err}
	}
}

// projectByName finds a project by its display name (e.g. "#Bizlink API").
func (m *model) projectByName(name string) (Project, bool) {
	for _, p := range m.projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

// deriveAll rebuilds allTasks and the project list from the cache, then re-views.
func (m *model) deriveAll() {
	if m.cache == nil {
		m.cache = newCache()
	}
	m.allTasks = m.cache.AllTasks()
	m.projects = m.cache.ProjectList()
	m.applyView()
}

// enqueue appends a command, persists the queue, and persists the cache.
func (m *model) enqueue(cmd Command) {
	m.queue = append(m.queue, cmd)
	SaveQueue(m.queue)
	m.cache.Save()
}

func (m *model) pendingNote() string {
	if n := len(m.queue); n > 0 {
		return fmt.Sprintf("• %d unsynced", n)
	}
	return ""
}

// ---------- optimistic local mutations (queued for sync) ----------

func (m *model) addTask(text string, proj Project) {
	q := parseQuickAdd(text)
	temp := "tmp-" + genID()
	it := apiItem{
		ID:        temp,
		Content:   q.Content,
		ProjectID: proj.ID,
		Priority:  q.Priority,
		Labels:    q.Labels,
		AddedAt:   nowStamp(),
	}
	if q.DueString != "" {
		it.Due = &apiDue{String: q.DueString}
	}
	m.cache.Items[temp] = it
	args := map[string]any{"content": q.Content, "priority": q.Priority}
	if proj.ID != "" {
		args["project_id"] = proj.ID
	}
	if len(q.Labels) > 0 {
		args["labels"] = q.Labels
	}
	if q.DueString != "" {
		args["due"] = map[string]any{"string": q.DueString}
	}
	m.enqueue(Command{Type: "item_add", UUID: genID(), TempID: temp, Args: args})
	m.deriveAll()
	m.status = "added: " + q.Content
}

func (m *model) completeTask(id, content string) {
	if it, ok := m.cache.Items[id]; ok {
		it.Checked = true
		m.cache.Items[id] = it
	}
	m.enqueue(Command{Type: "item_complete", UUID: genID(), Args: map[string]any{"id": id}})
	m.deriveAll()
	m.status = "completed: " + content
}

func (m *model) deleteTask(id, content string) {
	delete(m.cache.Items, id)
	m.enqueue(Command{Type: "item_delete", UUID: genID(), Args: map[string]any{"id": id}})
	m.deriveAll()
	m.status = "deleted: " + content
}

// updateItem mutates the cached item, queues an item_update, and refreshes.
func (m *model) updateItem(id string, mutate func(*apiItem), args map[string]any, status string) {
	if it, ok := m.cache.Items[id]; ok {
		mutate(&it)
		m.cache.Items[id] = it
	}
	args["id"] = id
	m.enqueue(Command{Type: "item_update", UUID: genID(), Args: args})
	m.deriveAll()
	m.refreshDetail()
	m.status = status
}

func (m *model) setPriorityDisplay(id string, p int) {
	api := 5 - p // display pN → API priority
	m.updateItem(id, func(it *apiItem) { it.Priority = api },
		map[string]any{"priority": api}, fmt.Sprintf("priority → p%d", p))
}

func (m *model) setDue(id, dueString string) {
	var dueArg any
	m.updateItem(id, func(it *apiItem) {
		if dueString == "" {
			it.Due = nil
		} else {
			it.Due = &apiDue{String: dueString}
		}
	}, map[string]any{}, "due date updated")
	if dueString != "" {
		dueArg = map[string]any{"string": dueString}
	}
	// re-queue with the right due arg (updateItem set id already on the last cmd)
	m.queue[len(m.queue)-1].Args["due"] = dueArg
	SaveQueue(m.queue)
}

func (m *model) setDeadline(id, date string) {
	var dlArg any
	if date != "" {
		dlArg = map[string]any{"date": date, "lang": "en"}
	}
	m.updateItem(id, func(it *apiItem) {
		if date == "" {
			it.Deadline = nil
		} else {
			it.Deadline = &apiDeadline{Date: date, Lang: "en"}
		}
	}, map[string]any{"deadline": dlArg}, "deadline updated")
}

func (m *model) setLabels(id, csv string) {
	labels := splitLabels(csv)
	m.updateItem(id, func(it *apiItem) { it.Labels = labels },
		map[string]any{"labels": labels}, "labels updated")
}

func (m *model) setContent(id, content string) {
	m.updateItem(id, func(it *apiItem) { it.Content = content },
		map[string]any{"content": content}, "name updated")
}

func (m *model) addCommentLocal(id, text string) {
	temp := "tmp-" + genID()
	m.cache.Notes[temp] = apiNote{ID: temp, ItemID: id, Content: text, PostedAt: nowStamp()}
	m.enqueue(Command{Type: "note_add", UUID: genID(), TempID: temp,
		Args: map[string]any{"item_id": id, "content": text}})
	m.comments = m.cache.CommentsFor(id)
	m.status = "comment added"
}

func (m *model) refreshDetail() {
	if m.detailID == "" || m.cache == nil {
		return
	}
	if it, ok := m.cache.Items[m.detailID]; ok {
		m.detailTask = m.cache.toTask(it)
	}
}

// splitLabels turns "a, b ,c" into ["a","b","c"] (dropping @ and blanks).
func splitLabels(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		f = strings.TrimPrefix(strings.TrimSpace(f), "@")
		if f != "" {
			out = append(out, f)
		}
	}
	return out
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
	if m.onlineView {
		items := make([]list.Item, len(m.onlineResults))
		for i, t := range m.onlineResults {
			items[i] = taskItem{t}
		}
		m.list.SetItems(items)
		return
	}
	if m.recentView {
		byID := make(map[string]Task, len(m.allTasks))
		for _, t := range m.allTasks {
			byID[t.ID] = t
		}
		var items []list.Item
		for _, id := range m.recentIDs {
			if t, ok := byID[id]; ok {
				items = append(items, taskItem{t})
			}
		}
		m.list.SetItems(items)
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.textQuery))
	today := todayStr()
	var matched []Task
	for _, t := range m.allTasks {
		if m.filter != "" && !EvalFilter(m.filter, t, today) {
			continue
		}
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

// dateSortKey makes an ISO-ish date string sortable; empty sorts last.
func dateSortKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "9999-99-99"
	}
	return s
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
		less = func(i, j int) bool { return dateSortKey(ts[i].DueDate) < dateSortKey(ts[j].DueDate) }
	case sortDeadline:
		less = func(i, j int) bool { return dateSortKey(ts[i].Deadline) < dateSortKey(ts[j].Deadline) }
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
	if m.onlineView {
		return fmt.Sprintf("online: %s — %d", m.onlineQuery, n)
	}
	if m.recentView {
		return fmt.Sprintf("recently added — %d", n)
	}
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
	m.recentView = false // any committed view leaves the recently-added view
	m.onlineView = false // …and the online-search view
	m.filter, m.textQuery, m.projectView, m.priorityView = s.filter, s.textQuery, s.projectView, s.priorityView
	m.applyView() // everything is local now
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
		m.input.Width = max(20, msg.Width-28) // visible typing width inside prompt boxes
		return m, nil

	case projectsLoadedMsg: // test seam
		m.projects = msg.projects
		m.setPickerItems()
		return m, nil

	case tasksLoadedMsg: // test seam: seed the task list directly
		m.filter = msg.filter
		m.allTasks = msg.tasks
		m.applyView()
		m.status = m.scopeStatus()
		return m, nil

	case tokenCheckedMsg:
		m.checking = false
		switch {
		case msg.valid:
			m.online = true
			m.status = "token OK — syncing…"
			m.syncing = true
			cmds := []tea.Cmd{syncNowCmd(m.cache.SyncToken, m.queue), m.restartAutoSync()}
			if m.mode == modeOnboard && !SettingsExist() {
				// first run: ask which label means "ongoing"
				m.beginLabelOnboard()
				cmds = append(cmds, textinput.Blink)
				return m, tea.Batch(cmds...)
			}
			if m.mode == modeOnboard {
				m.mode = modeList
				m.input.EchoMode = textinput.EchoNormal
				m.input.Blur()
				m.onboardErr = ""
			}
			return m, tea.Batch(cmds...)
		case msg.authErr:
			// token rejected → (re)onboard
			m.beginOnboard("That token was rejected. Paste a valid Todoist API token.")
			return m, textinput.Blink
		default:
			// network error: can't validate now
			m.online = false
			if m.mode == modeOnboard {
				// token saved but offline — let them in; it'll verify on sync
				m.mode = modeList
				m.input.EchoMode = textinput.EchoNormal
				m.input.Blur()
				m.onboardErr = ""
				m.status = "offline — token saved, will verify on next sync"
			} else {
				m.status = "offline — using cached data"
			}
			return m, nil
		}

	case onlineResultMsg:
		m.searching = false
		if msg.err != nil {
			m.err = "online search failed: " + msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.onlineView = true
		m.onlineQuery = msg.query
		m.onlineResults = make([]Task, len(msg.items))
		for i, it := range msg.items {
			m.onlineResults[i] = m.cache.toTask(it)
		}
		m.applyView()
		m.status = fmt.Sprintf("online: %s — %d", msg.query, len(m.onlineResults))
		return m, nil

	case autoSyncTickMsg:
		if msg.gen != m.tickGen || m.settings.SyncSeconds <= 0 {
			return m, nil // stale or disabled
		}
		cmds := []tea.Cmd{autoSyncCmd(m.tickGen, m.settings.SyncSeconds)} // reschedule
		if !m.syncing && HasToken() {
			m.syncing = true
			m.status = "auto-syncing…"
			cmds = append(cmds, syncNowCmd(m.cache.SyncToken, m.queue))
		}
		return m, tea.Batch(cmds...)

	case syncResultMsg:
		m.syncing = false
		if msg.err != nil {
			m.online = false
			m.err = "offline — changes queued (" + msg.err.Error() + ")"
			return m, nil
		}
		m.online = true
		m.err = ""
		m.cache.Merge(msg.resp)
		if msg.sent <= len(m.queue) { // drop flushed commands, keep any added during sync
			m.queue = m.queue[msg.sent:]
		} else {
			m.queue = nil
		}
		SaveQueue(m.queue)
		m.cache.Save()
		m.deriveAll()
		m.refreshDetail()
		if m.detailID != "" {
			m.comments = m.cache.CommentsFor(m.detailID)
		}
		m.status = "synced"
		return m, nil

	case actionMsg:
		m.status = msg.status
		return m, nil

	case errMsg:
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
		case modeCommentAdd:
			return m.updateCommentAdd(msg)
		case modePriorityPick:
			return m.updatePriorityPick(msg)
		case modeOnboard:
			return m.updateOnboard(msg)
		case modeOnboardLabel:
			return m.updateOnboardLabel(msg)
		case modeClearData:
			return m.updateClearData(msg)
		case modeOptions:
			return m.updateOptions(msg)
		case modeOptionsEdit:
			return m.updateOptionsEdit(msg)
		case modeOnlineSearch:
			return m.updateOnlineSearch(msg)
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
		// If already viewing a project, add straight into it.
		if m.projectView != "" {
			if p, ok := m.projectByName(m.projectView); ok {
				m.addProject = p
				m.mode = modeAdd
				m.err = ""
				m.input.EchoMode = textinput.EchoNormal
				m.input.Placeholder = "Buy milk @errand tomorrow 9am p1"
				m.input.SetValue("")
				m.input.Focus()
				return m, textinput.Blink
			}
		}
		// Otherwise open the project picker, pre-selecting the last-used project.
		m.mode = modeProjectPick
		m.pickIntent = pickAdd
		m.err = ""
		m.projQuery = ""
		m.setPickerItems()
		m.selectLastProject()
		return m, nil
	case "p":
		// View-by-project: prompt with the project list, then filter to it.
		m.mode = modeProjectPick
		m.pickIntent = pickView
		m.err = ""
		m.projQuery = ""
		m.setPickerItems()
		m.selectLastProject()
		return m, nil
	case "A":
		// Fast path: add straight to the most recent project, skipping the picker.
		if len(m.recents) == 0 {
			m.mode = modeProjectPick
			m.pickIntent = pickAdd
			m.projQuery = ""
			m.setPickerItems()
			m.selectLastProject()
			return m, nil
		}
		m.addProject = m.recents[0]
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
			m.commentErr = ""
			if m.cache != nil {
				m.comments = m.cache.CommentsFor(t.ID)
			}
		}
		return m, nil
	case "c":
		if t, ok := m.selectedTask(); ok {
			m.completeTask(t.ID, t.Content)
		}
		return m, nil
	case "o":
		// Ongoing — show all tasks tagged with the configured label.
		return m, m.commit(viewState{filter: "@" + m.settings.OngoingLabel})
	case "f":
		// Follow-up — show all tasks tagged with the configured label.
		return m, m.commit(viewState{filter: "@" + m.settings.FollowupLabel})
	case "T":
		// Tasks due today and earlier (today + overdue).
		return m, m.commit(viewState{filter: "today | overdue"})
	case "?":
		// Online search using Todoist's full filter grammar.
		m.mode = modeOnlineSearch
		m.err = ""
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "today · last 7 days · deadline before: +3 days · overdue & p1"
		m.input.SetValue(m.onlineQuery)
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "O":
		m.mode = modeOptions
		m.optCursor = 0
		m.err = ""
		return m, nil
	case "n":
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		return m, cmd
	case "v":
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		return m, cmd
	case "R":
		// Recently added — last 10 tasks by added date (from cache).
		m.recentView = true
		if m.cache != nil {
			m.recentIDs = m.cache.RecentTaskIDs(10)
		}
		m.applyView()
		m.status = fmt.Sprintf("recently added — %d", len(m.list.Items()))
		return m, nil
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
	case "x":
		if _, ok := m.selectedTask(); ok {
			m.mode = modeConfirm
			return m, nil
		}
	case "d":
		// Deadline is today.
		return m, m.commit(viewState{filter: "deadline today"})
	case "D":
		// Deadline is today or earlier.
		return m, m.commit(viewState{filter: "deadline overdue"})
	case "r":
		// Refresh the local view from the cache.
		m.deriveAll()
		m.status = m.scopeStatus()
		return m, nil
	case "s":
		// Sync: flush queued changes and pull updates.
		if m.syncing {
			return m, nil
		}
		m.syncing = true
		m.status = "syncing…"
		return m, syncNowCmd(m.cache.SyncToken, m.queue)
	case "b":
		return m, m.goBack()
	case "h", "esc":
		// Home — back to the all-projects / all-tasks view (undoable with b).
		return m, m.commit(viewState{})
	case "H":
		m.mode = modeHelp
		m.helpOffset = 0
		return m, nil
	case "X":
		m.mode = modeClearData
		return m, nil
	case "1":
		m.setSort(sortPriority)
		return m, nil
	case "2":
		m.setSort(sortDue)
		return m, nil
	case "3":
		m.setSort(sortDeadline)
		return m, nil
	case "4":
		m.setSort(sortProject)
		return m, nil
	case "5":
		m.setSort(sortName)
		return m, nil
	case "6":
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
	m.recentView = false // sorting leaves the recently-added view
	m.onlineView = false
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

// allProjectsID is the sentinel for the "All Projects" picker entry (view mode).
const allProjectsID = "__all__"

// setPickerItems rebuilds the project picker list: an optional "All Projects"
// reset (view mode), then up to 3 recently-chosen projects, a separator, then
// all projects.
func (m *model) setPickerItems() {
	var items []list.Item
	// When filtering, show a flat list of matching projects only.
	if q := strings.ToLower(strings.TrimSpace(m.projQuery)); q != "" {
		for _, p := range m.projects {
			if strings.Contains(strings.ToLower(p.Name), q) {
				items = append(items, projItem{p: p, kind: kindProject})
			}
		}
		m.projList.SetItems(items)
		return
	}
	if m.pickIntent == pickView {
		items = append(items, projItem{p: Project{ID: allProjectsID, Name: "↩ All Projects"}, kind: kindAllProjects})
	}
	if len(m.recents) > 0 {
		for _, p := range m.recents {
			items = append(items, projItem{p: p, kind: kindRecent})
		}
		items = append(items, projItem{kind: kindSep})
	}
	for _, p := range m.projects {
		items = append(items, projItem{p: p, kind: kindProject})
	}
	m.projList.SetItems(items)
}

// selectLastProject puts the cursor on the most recent project (the first
// recent row), so the default selection is the last project you chose.
func (m *model) selectLastProject() {
	for i, it := range m.projList.Items() {
		if p, ok := it.(projItem); ok && p.kind == kindRecent {
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
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.projQuery != "" { // first esc clears the filter
			m.projQuery = ""
			m.setPickerItems()
			return m, nil
		}
		m.mode = modeList
		return m, nil
	case "up", "down", "ctrl+p", "ctrl+n", "pgup", "pgdown":
		var cmd tea.Cmd
		m.projList, cmd = m.projList.Update(msg)
		return m, cmd
	case "backspace":
		if m.projQuery != "" {
			r := []rune(m.projQuery)
			m.projQuery = string(r[:len(r)-1])
			m.setPickerItems()
			m.projList.Select(0)
		}
		return m, nil
	case "enter":
		it, ok := m.projList.SelectedItem().(projItem)
		if !ok || it.kind == kindSep {
			return m, nil // nothing selectable
		}
		if m.pickIntent == pickView {
			m.mode = modeList
			if it.p.ID == allProjectsID {
				return m, m.commit(viewState{}) // back to all projects
			}
			m.recents = pushRecentProject(m.recents, it.p)
			SaveRecentProjects(m.recents)
			return m, m.commit(viewState{projectView: it.p.Name})
		}
		// pickAdd: remember the project and move to the add input.
		m.addProject = it.p
		m.recents = pushRecentProject(m.recents, it.p)
		SaveRecentProjects(m.recents)
		m.mode = modeAdd
		m.input.Placeholder = "Buy milk @errand tomorrow 9am p1"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	}
	// Any printable input narrows the list (type-to-filter).
	if len(msg.Runes) > 0 {
		m.projQuery += string(msg.Runes)
		m.setPickerItems()
		m.projList.Select(0)
	}
	return m, nil
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
			m.addTask(val, m.addProject)
			return m, nil
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
		m.completeTask(m.detailID, m.detailTask.Content)
		return m, nil
	case "1", "2", "3", "4":
		p := int(msg.String()[0] - '0')
		m.setPriorityDisplay(m.detailID, p)
		return m, nil
	case "t":
		return m, m.startEdit(efDate, "today · tomorrow 9am · every monday · (empty clears)", "")
	case "D":
		return m, m.startEdit(efDeadline, "YYYY-MM-DD · (empty clears)", m.detailTask.Deadline)
	case "l":
		return m, m.startEdit(efLabels, "comma-separated, e.g. ongoing,follow-up", labelsCSV(m.detailTask.Labels))
	case "e":
		return m, m.startEdit(efContent, "task name", m.detailTask.Content)
	case "m":
		m.mode = modeCommentAdd
		m.input.Placeholder = "write a comment…"
		m.input.SetValue("")
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateCommentAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeDetail
		m.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		m.mode = modeDetail
		m.input.Blur()
		if val == "" {
			return m, nil
		}
		m.addCommentLocal(m.detailID, val)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
		switch m.editField {
		case efDate:
			m.setDue(m.detailID, val)
		case efDeadline:
			m.setDeadline(m.detailID, val)
		case efLabels:
			m.setLabels(m.detailID, val)
		case efContent:
			if val != "" {
				m.setContent(m.detailID, val)
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateClearData(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Clear everything local: token files, cache, and queued changes.
		ClearToken()
		ClearLocalData()
		m.cache = newCache()
		m.queue = nil
		m.online = false
		m.history = nil
		m.current = viewState{}
		m.deriveAll()
		if TokenFromEnv() {
			m.mode = modeList
			m.status = "cleared cache & saved token (but $TODOIST_API_TOKEN is still set)"
			return m, nil
		}
		m.beginOnboard("All local data cleared. Paste a Todoist API token to reconnect.")
		return m, textinput.Blink
	default:
		m.mode = modeList
		return m, nil
	}
}

func (m model) updateOnlineSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "enter":
		q := strings.TrimSpace(m.input.Value())
		m.mode = modeList
		m.input.Blur()
		if q == "" {
			return m, nil
		}
		m.searching = true
		m.status = "searching Todoist…"
		return m, onlineSearchCmd(q)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateOnboardLabel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "esc":
		label := strings.TrimPrefix(strings.TrimSpace(m.input.Value()), "@")
		if label == "" {
			label = "ongoing"
		}
		m.settings.OngoingLabel = label
		m.settings.Save() // writes the file → marks first-run complete
		m.mode = modeList
		m.input.Blur()
		m.status = "ready — ongoing label: @" + label
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// optionRows describes the editable settings, in display order.
func (m model) optionRows() []struct{ label, value string } {
	sync := "off"
	if m.settings.SyncSeconds > 0 {
		sync = fmt.Sprintf("%d seconds", m.settings.SyncSeconds)
	}
	return []struct{ label, value string }{
		{"Ongoing label", "@" + m.settings.OngoingLabel},
		{"Follow-up label", "@" + m.settings.FollowupLabel},
		{"Background auto-sync", sync},
	}
}

func (m model) updateOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.optionRows()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "O":
		m.mode = modeList
		return m, nil
	case "up", "k":
		if m.optCursor > 0 {
			m.optCursor--
		}
		return m, nil
	case "down", "j":
		if m.optCursor < len(rows)-1 {
			m.optCursor++
		}
		return m, nil
	case "enter":
		m.mode = modeOptionsEdit
		m.input.EchoMode = textinput.EchoNormal
		switch m.optCursor {
		case 0:
			m.input.Placeholder = "ongoing"
			m.input.SetValue(m.settings.OngoingLabel)
		case 1:
			m.input.Placeholder = "ffup"
			m.input.SetValue(m.settings.FollowupLabel)
		case 2:
			m.input.Placeholder = "seconds (0 = off)"
			m.input.SetValue(strconv.Itoa(m.settings.SyncSeconds))
		}
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateOptionsEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeOptions
		m.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		var restart tea.Cmd
		switch m.optCursor {
		case 0:
			label := strings.TrimPrefix(val, "@")
			if label == "" {
				label = "ongoing"
			}
			m.settings.OngoingLabel = label
		case 1:
			label := strings.TrimPrefix(val, "@")
			if label == "" {
				label = "ffup"
			}
			m.settings.FollowupLabel = label
		case 2:
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				n = 0
			}
			if n > 0 && n < 30 {
				n = 30 // avoid hammering the API
			}
			m.settings.SyncSeconds = n
			restart = m.restartAutoSync()
		}
		m.settings.Save()
		m.mode = modeOptions
		m.input.Blur()
		m.status = "settings saved"
		return m, restart
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateOnboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			m.onboardErr = "Please paste a token (or Esc to quit)."
			return m, nil
		}
		if err := SaveToken(val); err != nil {
			m.onboardErr = "Couldn't save token: " + err.Error()
			return m, nil
		}
		m.onboardErr = "Checking token…"
		m.checking = true
		m.input.SetValue("")
		return m, checkTokenCmd()
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
			m.deleteTask(t.ID, t.Content)
		}
		return m, nil
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
	if m.onlineView {
		scope = "  online: " + m.onlineQuery
	} else if m.recentView {
		scope = "  recently added"
	} else if m.filter != "" {
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

	if m.mode == modeOnboard {
		return m.onboardView(header)
	}

	if m.mode == modeOnboardLabel {
		dim := lipgloss.NewStyle().Foreground(subColor)
		accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
		box := promptBox.Render(accent.Render("Ongoing label  @") + m.input.View())
		body := lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+accent.Render("One more thing…"),
			"",
			"  "+dim.Render("Which label marks a task as “ongoing”? The o key will"),
			"  "+dim.Render("show every task with this label. Default: ongoing."),
			"",
			"  "+box,
			"",
			helpStyle.Render("  enter save · (you can change this later in Options)"),
		)
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	if m.mode == modeOptions || m.mode == modeOptionsEdit {
		return m.optionsView(header)
	}

	if m.mode == modeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.helpView())
	}

	if m.mode == modeDetail || m.mode == modeDetailEdit || m.mode == modeCommentAdd {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.detailView())
	}

	// Confirmation dialogs render as their own modal screen (no list behind).
	if m.mode == modeConfirm {
		t, _ := m.selectedTask()
		name := t.Content
		if len(name) > 60 {
			name = name[:59] + "…"
		}
		accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
		dim := lipgloss.NewStyle().Foreground(subColor)
		box := promptBox.Render(lipgloss.JoinVertical(lipgloss.Left,
			accent.Render("Delete this task?"),
			dim.Render("\""+name+"\" — this can't be undone."),
			"",
			accent.Render("y")+dim.Render(" delete    ")+accent.Render("n")+dim.Render(" cancel"),
		))
		return lipgloss.JoinVertical(lipgloss.Left, header, "", "  "+box)
	}
	if m.mode == modeClearData {
		accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
		dim := lipgloss.NewStyle().Foreground(subColor)
		rows := []string{
			accent.Render("Clear all local data?"),
			dim.Render("This signs todoui out and wipes its local state:"),
			dim.Render("  • your saved Todoist API token"),
			dim.Render("  • the offline cache (tasks, projects, comments)"),
			dim.Render("  • any changes not yet synced"),
			dim.Render("You'll be asked for your token again to reconnect."),
		}
		if n := len(m.queue); n > 0 {
			rows = append(rows, "", lipgloss.NewStyle().Foreground(lipgloss.Color("#EB8909")).
				Render(fmt.Sprintf("⚠ %d unsynced change(s) will be LOST — press n, then s to sync first.", n)))
		}
		rows = append(rows, "", accent.Render("y")+dim.Render(" clear everything    ")+accent.Render("n")+dim.Render(" cancel"))
		box := promptBox.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		return lipgloss.JoinVertical(lipgloss.Left, header, "", "  "+box)
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
		line := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(prompt)
		if m.projQuery != "" {
			line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Render("filter: "+m.projQuery+"▌")
		}
		hint := line
		help := helpStyle.Render("type to filter · ↑/↓ move · enter select · esc clear/cancel")
		picker := lipgloss.JoinVertical(lipgloss.Left, hint, m.projList.View(), help)
		return lipgloss.JoinVertical(lipgloss.Left, header, picker)
	case modeAdd:
		proj := m.addProject.Name
		if proj == "" {
			proj = "#Inbox"
		}
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).
			Render("Add → " + proj + "  ")
		body = promptBox.Render(label + m.input.View())
	case modeSearch:
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("Search  ")
		body = promptBox.Render(label + m.input.View())
	case modeOnlineSearch:
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("Todoist search (online)  ")
		body = promptBox.Render(label + m.input.View())
	}

	footer := m.footer()

	if body != "" {
		// Shrink the list so the prompt box doesn't push content off the top.
		h := m.height - lipgloss.Height(header) - lipgloss.Height(body) - lipgloss.Height(footer)
		if h < 3 {
			h = 3
		}
		m.list.SetHeight(h)
		return lipgloss.JoinVertical(lipgloss.Left, header, body, m.list.View(), footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.list.View(), footer)
}

// optionsView renders the settings page.
func (m model) optionsView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	val := lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8"))
	rows := m.optionRows()

	lines := []string{"", "  " + accent.Render("Options"), ""}
	for i, r := range rows {
		cur := "   "
		name := dim.Render(fmt.Sprintf("%-22s", r.label))
		if i == m.optCursor && m.mode == modeOptions {
			cur = accent.Render(" ▸ ")
			name = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(fmt.Sprintf("%-22s", r.label))
		}
		lines = append(lines, cur+name+val.Render(r.value))
	}
	lines = append(lines, "")

	if m.mode == modeOptionsEdit {
		titles := []string{"Ongoing label  @", "Follow-up label  @", "Auto-sync seconds (0 = off)  "}
		box := promptBox.Render(accent.Render(titles[m.optCursor]) + m.input.View())
		lines = append(lines, "  "+box, "", helpStyle.Render("  enter save · esc cancel"))
	} else {
		lines = append(lines, dim.Render("  The ongoing label is what the o key filters on."))
		lines = append(lines, dim.Render("  Auto-sync pushes queued changes & pulls on a timer."))
		lines = append(lines, "", helpStyle.Render("  ↑/↓ move · enter edit · esc/O close"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
}

// onboardView renders the first-run / invalid-token token entry screen.
func (m model) onboardView(header string) string {
	dim := lipgloss.NewStyle().Foreground(subColor)
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	lines := []string{
		"",
		"  " + accent.Render("Connect to Todoist"),
		"",
		"  " + dim.Render("Get your API token from Todoist →"),
		"  " + dim.Render("Settings → Integrations → Developer → API token."),
		"  " + dim.Render("It's saved to ~/.config/todoui/config.json."),
		"",
	}
	if m.checking {
		lines = append(lines, "  "+statusStyle.Render("⟳ checking token…"))
	} else {
		box := promptBox.Width(min(m.width-4, 64)).Render(accent.Render("Token  ") + m.input.View())
		lines = append(lines, box)
	}
	if m.onboardErr != "" {
		style := dim
		if strings.Contains(strings.ToLower(m.onboardErr), "reject") || strings.Contains(m.onboardErr, "Couldn't") {
			style = lipgloss.NewStyle().Foreground(brandRed)
		}
		lines = append(lines, "", "  "+style.Render(m.onboardErr))
	}
	lines = append(lines, "", helpStyle.Render("  enter save & check · esc quit"))
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
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
		field("Deadline", t.Deadline, lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))),
		field("Project", t.Project, lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8"))),
		field("Labels", t.Labels, lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379"))),
		field("ID", t.ID, label),
		"",
	}

	// Comments section.
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("#8AB4F8")).Bold(true)
	count := ""
	if m.commentErr == "" {
		count = fmt.Sprintf(" (%d)", len(m.comments))
	}
	lines = append(lines, "  "+head.Render("Comments"+count))
	switch {
	case m.commentErr != "":
		lines = append(lines, "  "+errStyle.Render("⚠ "+m.commentErr))
	case len(m.comments) == 0:
		lines = append(lines, "  "+label.Render("(none yet)"))
	default:
		for _, c := range m.comments {
			when := lipgloss.NewStyle().Foreground(subColor).Render(shortTime(c.PostedAt))
			body := strings.ReplaceAll(strings.TrimSpace(c.Content), "\n", " ")
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(brandRed).Render("• ")+when+"  "+lipgloss.NewStyle().Foreground(lipgloss.Color("#DDDDDD")).Render(body))
		}
	}
	lines = append(lines, "")

	if m.mode == modeDetailEdit {
		titles := map[editField]string{
			efDate:     "Set due date",
			efDeadline: "Set deadline (YYYY-MM-DD)",
			efLabels:   "Set labels (comma-separated)",
			efContent:  "Edit name",
		}
		prompt := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(titles[m.editField] + "  ")
		box := promptBox.Render(prompt + m.input.View())
		lines = append(lines, box, "", helpStyle.Render("  enter save · esc cancel"))
	} else if m.mode == modeCommentAdd {
		prompt := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("New comment  ")
		box := promptBox.Render(prompt + m.input.View())
		lines = append(lines, box, "", helpStyle.Render("  enter post · esc cancel"))
	} else {
		actions := "  " +
			key.Render("1-4") + label.Render(" priority  ") +
			key.Render("t") + label.Render(" due  ") +
			key.Render("D") + label.Render(" deadline  ") +
			key.Render("l") + label.Render(" labels  ") +
			key.Render("e") + label.Render(" name  ") +
			key.Render("m") + label.Render(" comment  ") +
			key.Render("c") + label.Render(" done  ") +
			key.Render("b") + label.Render(" back")
		lines = append(lines, actions)
		if m.err != "" {
			lines = append(lines, "", errStyle.Render("⚠ "+m.err))
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
		row("n / v", "Next page / previous page (also pgdn/pgup)"),
		row("b", "Back — return to the previous view (like a browser)"),
		row("h / esc", "Home — back to all tasks / all projects"),
		row("q ctrl+c", "Quit"),
		"",
		head.Render("  Tasks"),
		row("enter", "Open the task — view & edit due, deadline, priority, labels, name"),
		row("a", "Add a task — choose the project first"),
		row("A", "Add a task straight to the most recent project"),
		row("c", "Complete the selected task"),
		row("x", "Delete the selected task (asks first)"),
		row("r", "Refresh the view from the local cache"),
		row("s", "Sync — push queued changes & pull updates"),
		"",
		head.Render("  Offline & settings"),
		row("", "Changes apply instantly to a local cache and queue up."),
		row("", "Press s to push them; everything works offline until then."),
		row("O", "Options — ongoing label & background-sync interval"),
		row("X", "Clear data — remove token, cache & queue (asks first)"),
		"",
		head.Render("  In the task view"),
		row("1-4", "Set priority (p1–p4)"),
		row("t", "Set due date    D  set deadline    l  set labels    e  edit name"),
		row("m", "Add a comment (view existing comments above)"),
		row("c", "Complete    b/esc  back to the list"),
		"",
		head.Render("  Views & filters"),
		row("p", "View by project (pick from the list; “↩ All Projects” to reset)"),
		row("P", "Filter by priority (pick p1–p4 from the menu)"),
		row("o", "Ongoing — tasks with your ongoing label (set in Options)"),
		row("f", "Follow-up — tasks with your follow-up label (set in Options)"),
		row("T", "Due today or earlier (today + overdue)"),
		row("d", "Deadline is today"),
		row("D", "Deadline is today or earlier"),
		row("R", "Recently added — the last 10 tasks you created"),
		row("/", "Search — plain words; or a local filter (today, #x & p1)"),
		row("?", "Online search — full Todoist filter grammar (needs network)"),
		"",
		head.Render("  Sort  (press the same number again to reverse)"),
		row("1", "Priority (p1 → p4)"),
		row("2", "Due date (soonest first, no-date last)"),
		row("3", "Deadline (soonest first, none last)"),
		row("4", "Project (A → Z)"),
		row("5", "Name (A → Z)"),
		row("6", "Labels (A → Z)"),
		row("0", "Default Todoist order"),
		"",
		head.Render("  Search tips"),
		row("plain text", "groceries, call mom — instant local search of name/project/labels"),
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
	// status line with sync state
	st := m.status
	if m.syncing {
		st = "⟳ syncing… " + st
	}
	var badges []string
	if n := len(m.queue); n > 0 {
		badges = append(badges, lipgloss.NewStyle().Foreground(lipgloss.Color("#EB8909")).Render(fmt.Sprintf("●%d unsynced", n)))
	}
	if m.online {
		badges = append(badges, lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379")).Render("online"))
	}
	statusLine := statusStyle.Render(st)
	if len(badges) > 0 {
		statusLine = statusStyle.Render(st+"  ") + strings.Join(badges, statusStyle.Render(" · "))
	}
	if m.err != "" {
		statusLine = errStyle.Render("⚠ " + m.err)
	}

	keys := "a add · enter view · c done · x del · / find · ? online · p project · T today · o ongoing · f follow-up · O options · s sync · H help · q quit"
	right := helpStyle.Render(keys)
	gap := m.width - lipgloss.Width(statusLine) - lipgloss.Width(right)
	if gap < 1 {
		return lipgloss.JoinVertical(lipgloss.Left, statusLine, right)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, statusLine, strings.Repeat(" ", gap), right)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("todoui", version)
			return
		case "--help", "-h", "help":
			fmt.Println("todoui — a terminal UI for Todoist (Sync API, offline-first)")
			fmt.Println("Usage: todoui            start the UI")
			fmt.Println("       todoui sync       flush queued changes + pull, headless")
			fmt.Println("       todoui --version  print version")
			fmt.Println("Press H inside the app for the keyboard reference.")
			return
		case "sync":
			runHeadlessSync()
			return
		}
	}
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
	}
}

// runHeadlessSync flushes the queue and pulls updates without the UI.
func runHeadlessSync() {
	cache := LoadCache()
	queue := LoadQueue()
	sent := len(queue)
	resp, err := DoSync(cache.SyncToken, queue)
	if err != nil {
		fmt.Println("sync failed:", err)
		os.Exit(1)
	}
	cache.Merge(resp)
	cache.Save()
	SaveQueue(nil)
	fmt.Printf("synced: flushed %d change(s); %d tasks, %d projects cached\n",
		sent, len(cache.Items), len(cache.Projects))
}
