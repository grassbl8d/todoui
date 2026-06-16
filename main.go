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

// version is the default shown in-app; release builds override it with
// -ldflags "-X main.version=vX.Y.Z". Bump this when cutting a new version.
var version = "v0.2.1"

// ---------- theming (light / dark) ----------

// Theme holds every colour the UI uses, so we can swap light/dark at runtime.
type Theme struct {
	Accent, Dim, Sub       lipgloss.Color
	Text, Bright           lipgloss.Color
	P1, P2, P3, P4         lipgloss.Color
	Project, Due, Deadline lipgloss.Color
	Labels, Warn           lipgloss.Color
	TitleFg, TitleBg       lipgloss.Color
}

var darkTheme = Theme{
	Accent: "#E44332", Dim: "#6C6C6C", Sub: "#9A9A9A",
	Text: "#DDDDDD", Bright: "#FFFFFF",
	P1: "#E44332", P2: "#EB8909", P3: "#246FE0", P4: "#808080",
	Project: "#8AB4F8", Due: "#E5C07B", Deadline: "#E06C75",
	Labels: "#98C379", Warn: "#EB8909",
	TitleFg: "#FFFFFF", TitleBg: "#E44332",
}

var lightTheme = Theme{
	Accent: "#C5341F", Dim: "#6B7280", Sub: "#4B5563",
	Text: "#1F2937", Bright: "#000000",
	P1: "#C5341F", P2: "#B45309", P3: "#1D4ED8", P4: "#6B7280",
	Project: "#1D4ED8", Due: "#B45309", Deadline: "#B91C1C",
	Labels: "#15803D", Warn: "#B45309",
	TitleFg: "#FFFFFF", TitleBg: "#E44332",
}

// Active palette (set by applyTheme) used throughout the views.
var (
	th            Theme
	brandRed      lipgloss.Color
	dimColor      lipgloss.Color
	subColor      lipgloss.Color
	textColor     lipgloss.Color
	brightColor   lipgloss.Color
	projectColor  lipgloss.Color
	dueColor      lipgloss.Color
	deadlineColor lipgloss.Color
	labelColor    lipgloss.Color
	warnColor     lipgloss.Color
	prioColors    map[string]lipgloss.Color

	titleBarStyle lipgloss.Style
	statusStyle   lipgloss.Style
	errStyle      lipgloss.Style
	helpStyle     lipgloss.Style
	promptBox     lipgloss.Style
)

// applyTheme makes t the active palette and rebuilds the shared styles.
func applyTheme(t Theme) {
	th = t
	brandRed, dimColor, subColor = t.Accent, t.Dim, t.Sub
	textColor, brightColor = t.Text, t.Bright
	projectColor, dueColor, deadlineColor = t.Project, t.Due, t.Deadline
	labelColor, warnColor = t.Labels, t.Warn
	prioColors = map[string]lipgloss.Color{"p1": t.P1, "p2": t.P2, "p3": t.P3, "p4": t.P4}
	titleBarStyle = lipgloss.NewStyle().Background(t.TitleBg).Foreground(t.TitleFg).Bold(true).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(t.Sub).Padding(0, 1)
	errStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Padding(0, 1)
	helpStyle = lipgloss.NewStyle().Foreground(t.Dim).Padding(0, 1)
	promptBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(0, 1)
}

func init() { applyTheme(darkTheme) } // sensible default before settings load

// ---------- list item ----------

type taskItem struct {
	t     Task
	sep   bool   // a non-selectable separator row (e.g. "completed")
	label string // separator label
}

func (i taskItem) FilterValue() string {
	if i.sep {
		return ""
	}
	return i.t.Content + " " + i.t.Project + " " + i.t.Labels
}

// taskDelegate renders each task across two lines with a priority-coloured marker.
type taskDelegate struct{}

func (d taskDelegate) Height() int                         { return 2 }
func (d taskDelegate) Spacing() int                        { return 1 }
func (d taskDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d taskDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(taskItem)
	if !ok {
		return
	}
	if it.sep {
		// Read-only divider between active and completed tasks.
		bar := strings.Repeat("─", 14)
		fmt.Fprintf(w, "%s\n",
			lipgloss.NewStyle().Foreground(dimColor).Render("  "+bar+" "+it.label+" "+bar))
		return
	}
	t := it.t
	selected := index == m.Index()

	pc := prioColors[t.Priority]
	if pc == "" {
		pc = prioColors["p4"]
	}

	// Completed tasks render dimmed with a ✓ and struck-through text (read-only).
	if t.Done {
		mk := "  "
		ts := lipgloss.NewStyle().Foreground(subColor).Strikethrough(true)
		if selected {
			mk = lipgloss.NewStyle().Foreground(dueColor).Bold(true).Render("▌ ")
			ts = ts.Foreground(textColor)
		}
		check := lipgloss.NewStyle().Foreground(dueColor).Render("✓")
		var meta []string
		if t.Project != "" {
			meta = append(meta, t.Project)
		}
		if strings.TrimSpace(t.Labels) != "" {
			meta = append(meta, t.Labels)
		}
		line1 := fmt.Sprintf("%s%s  %s", mk, check, ts.Render(t.Content))
		line2 := "    " + lipgloss.NewStyle().Foreground(dimColor).Render(strings.Join(meta, "  ·  "))
		fmt.Fprintf(w, "%s\n%s", line1, line2)
		return
	}

	marker := "  "
	titleStyle := lipgloss.NewStyle().Foreground(textColor)
	if selected {
		marker = lipgloss.NewStyle().Foreground(pc).Bold(true).Render("▌ ")
		titleStyle = titleStyle.Foreground(brightColor).Bold(true)
	}

	prio := lipgloss.NewStyle().Foreground(pc).Bold(true).Render(t.Priority)

	// meta line: #project · due · @labels
	var meta []string
	if t.Project != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(projectColor).Render(t.Project))
	}
	if strings.TrimSpace(t.DueDate) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(dueColor).Render(fmtDate(t.DueDate)))
	}
	if strings.TrimSpace(t.Deadline) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(deadlineColor).Render("⚑ "+fmtDate(t.Deadline)))
	}
	if strings.TrimSpace(t.Labels) != "" {
		meta = append(meta, lipgloss.NewStyle().Foreground(labelColor).Render(t.Labels))
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
		base = dueColor
		prefix = "★ "
	case kindAllProjects:
		base = dueColor
	default:
		base = projectColor
	}
	style := lipgloss.NewStyle().Foreground(base)
	if selected {
		prefix = lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("▸ ")
		style = lipgloss.NewStyle().Foreground(brightColor).Bold(true)
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
	modeDetail            // viewing a single task
	modeDetailEdit        // editing one field of the task in detail view
	modeCommentAdd        // writing a new comment in the detail view
	modePriorityPick      // choosing a priority to filter by
	modeOnboard           // first-run / invalid-token: prompt for the API token
	modeOnboardLabel      // first-run: choose the "ongoing" label
	modeClearData         // confirm clearing token + cache + queue
	modeOptions           // settings page
	modeOptionsEdit       // editing one setting
	modeOnlineSearch      // online Todoist filter query (?)
	modeCommand           // ":" command line (e.g. :unpin)
	modeProjectAdd        // entering a new project name (in the picker)
	modeProjectDelete     // confirm deleting a project (in the picker)
	modeIdeaAdd           // 💡 capture a new idea (overlay)
	modeIdeaList          // 💡 browse captured ideas
	modeIdeaRename        // 💡 rename the selected idea
	modeDeadlinePick      // pick a deadline from quick options
	modeTimezone          // searchable IANA timezone picker
	modePalette           // ` quick-action palette: search & run any command
	modeAbout             // ~ about screen (logo + contributors)
	modeMindMap           // 🗺 navigating an idea's mind map
	modeMindEdit          // 🗺 typing a mind-map node's text
	modeMindHelp          // 🗺 dedicated keyboard help for mind-map mode
	modeMindPalette       // 🗺 ` quick-action palette for mind-map mode
	modeMindConfirmDelete // 🗺 y/n confirm before deleting a node
	modeMindConfirmUnbind // 🗺 y/n confirm before unbinding the project
)

// deadlineOptions are the quick picks shown when setting a deadline.
var deadlineOptions = []struct{ label, phrase string }{
	{"Today", "today"},
	{"Tomorrow", "tomorrow"},
	{"This weekend (Sat)", "saturday"},
	{"Next week", "next week"},
	{"End of month", "end of month"},
	{"Next month", "next month"},
	{"Custom date…", "custom"},
	{"Clear deadline", "clear"},
}

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
	sortAdded                    // date added (newest first)
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
	case sortAdded:
		return "date added"
	default:
		return "default"
	}
}

// pickIntent distinguishes why the project picker is open.
type pickIntent int

const (
	pickAdd       pickIntent = iota // choosing a project to add a task into
	pickView                        // choosing a project to filter the view by
	pickMindTasks                   // choosing a project for a mind map's task nodes
)

type model struct {
	list          list.Model
	projList      list.Model
	input         textinput.Model
	mode          mode
	pickIntent    pickIntent  // what the project picker is for (add vs view)
	projects      []Project   // all projects (source for the picker)
	allTasks      []Task      // full set from the last server load
	filter        string      // active server-side Todoist filter (working value)
	textQuery     string      // local case-insensitive text search (working value)
	projectView   string      // local project filter, display name e.g. "#Bills" (working value)
	priorityView  string      // local priority filter "p1".."p4" (working value)
	prioCursor    int         // cursor in the priority picker
	cache         *Cache      // local offline-first snapshot
	queue         []Command   // pending Sync API commands (flushed on sync)
	syncing       bool        // a sync is in flight
	syncBg        bool        // the in-flight sync is a background auto-sync
	spinFrame     int         // animation frame for the sync progress bar
	completedView int         // 0 active only · 1 active+completed · 2 completed only
	online        bool        // last sync succeeded
	current       viewState   // the committed view currently shown
	history       []viewState // back-stack of previously committed views
	sortMode      sortMode    // current ordering of the task list
	sortDesc      bool        // reverse the current ordering
	detailTask    Task        // task shown in the detail view
	detailID      string      // id of the task in the detail view
	editField     editField   // which field the detail editor is editing
	comments      []Comment   // comments for the task in the detail view
	commentErr    string      // error from the last comment fetch/post
	recentView    bool        // showing the recently-added tasks
	recentIDs     []string    // task IDs in recently-added order
	onlineView    bool        // showing online (server filter) search results
	onlineResults []Task      // results of the last online search
	onlineQuery   string      // last online query (for the header)
	searching     bool        // online search in flight
	onboardErr    string      // error shown on the onboarding screen
	checking      bool        // validating the token
	projQuery     string      // type-to-filter text in the project picker
	settings      Settings    // user preferences (ongoing label, sync interval)
	tickGen       int         // generation guard for the auto-sync ticker
	optCursor     int         // selected row on the options page
	tzAll         []string    // all selectable IANA zone names (loaded on first open)
	tzQuery       string      // type-to-filter text in the timezone picker
	tzCursor      int         // selected row in the timezone picker
	palQuery      string      // type-to-filter text in the ` quick-action palette
	palCursor     int         // selected row in the quick-action palette
	pinnedID      string      // when set, only this task is shown (focus mode)
	showComments  bool        // on the pinned focus screen, show the comments list
	projDelTarget Project     // project pending delete-confirmation
	ideas         []Idea      // locally-captured ideas (newest first)
	ideaCursor    int         // selected row in the idea list
	mindIdea      int         // index into ideas of the mind map being edited
	mindCursor    int         // selected row in the flattened mind-map tree
	mindEditNode  *MindNode   // node whose text is being edited (nil = the root idea)
	mindEditNew   bool        // the edited node was just created (esc/empty discards it)
	mindDelTarget *MindNode   // node pending delete-confirmation in the mind map
	dlCursor      int         // selected row in the deadline picker
	homeFlash     bool        // brief highlight of the home/clear hint when pressed
	helpOffset    int         // scroll offset of the help page
	addProject    Project     // project chosen for the task currently being added
	recents       []Project   // recently-chosen projects, most recent first (persisted)
	status        string
	err           string
	width         int
	height        int
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
type homeFlashOffMsg struct{}
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
		ideas:    LoadIdeas(),
		status:   "ready",
	}
	m.applyThemeFromSettings()
	dateFmt = m.settings.DateFormat
	applyTimezone(m.settings.Timezone)
	m.deriveAll()
	if !HasToken() {
		m.beginOnboard("Welcome! Paste your Todoist API token to get started.")
	}
	return m
}

// applyThemeFromSettings selects light or dark per the saved preference.
func (m *model) applyThemeFromSettings() {
	if m.settings.Light {
		applyTheme(lightTheme)
	} else {
		applyTheme(darkTheme)
	}
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

// startSync kicks off a foreground sync (flush queue + pull) with the animated
// progress bar. No-op (nil cmd) if a sync is already running.
func (m *model) startSync() tea.Cmd {
	if m.syncing {
		return nil
	}
	m.syncing = true
	m.syncBg = false
	m.spinFrame = 0
	m.status = "syncing…"
	return tea.Batch(syncNowCmd(m.cache.SyncToken, m.queue), spinnerTick())
}

// syncCyan is the accent used for the in-progress sync bar.
var syncCyan = lipgloss.Color("#22d3ee")

// spinnerTickMsg advances the sync progress-bar animation.
type spinnerTickMsg struct{}

// spinnerTick schedules the next animation frame.
func spinnerTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

// syncBar renders a full-width indeterminate progress bar: a solid cyan block
// sweeping across a dim track, with a label. Shown only while a sync is running.
func (m model) syncBar() string {
	label := " ⟳ syncing "
	if m.syncBg {
		label = " ⟳ auto-sync "
	}
	labelStyled := lipgloss.NewStyle().Foreground(syncCyan).Bold(true).Render(label)
	barW := m.width - lipgloss.Width(label)
	if barW < 4 {
		return labelStyled
	}
	seg := barW / 4
	if seg < 3 {
		seg = 3
	}
	// Bounce the block back and forth across the track.
	span := barW - seg
	if span < 1 {
		span = 1
	}
	p := m.spinFrame % (2 * span)
	if p > span {
		p = 2*span - p
	}
	track := lipgloss.NewStyle().Background(dimColor)
	block := lipgloss.NewStyle().Background(syncCyan)
	sp := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat(" ", n)
	}
	bar := track.Render(sp(p)) + block.Render(sp(seg)) + track.Render(sp(barW-seg-p))
	return labelStyled + bar
}

func onlineSearchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		items, err := FilterTasks(query)
		return onlineResultMsg{query: query, items: items, err: err}
	}
}

// completedFetchedMsg carries server-side completed tasks for the completed view.
type completedFetchedMsg struct {
	items []apiItem
	err   error
}

// fetchCompletedCmd pulls completed tasks for a project from Todoist.
func fetchCompletedCmd(projectID string) tea.Cmd {
	return func() tea.Msg {
		items, err := FetchCompletedTasks(projectID)
		return completedFetchedMsg{items: items, err: err}
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

// addTask optimistically adds a task and returns its temporary id (resolved to a
// real id on the next sync via the temp-id mapping).
func (m *model) addTask(text string, proj Project) string {
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
	return temp
}

func (m *model) completeTask(id, content string) {
	if it, ok := m.cache.Items[id]; ok {
		it.Checked = true
		m.cache.Items[id] = it
	}
	m.enqueue(Command{Type: "item_complete", UUID: genID(), Args: map[string]any{"id": id}})
	m.unpinIfMatches(id)
	m.deriveAll()
	m.status = "completed: " + content
}

// uncompleteTask reopens a completed task (queued for sync) and re-links any
// mind-map node pointing at it.
func (m *model) uncompleteTask(id, content string) {
	if it, ok := m.cache.Items[id]; ok {
		it.Checked = false
		m.cache.Items[id] = it
	}
	m.enqueue(Command{Type: "item_uncomplete", UUID: genID(), Args: map[string]any{"id": id}})
	m.syncMindLinks(nil) // clear the Done flag on any linked node
	m.deriveAll()
	m.status = "reopened: " + content
}

func (m *model) deleteTask(id, content string) {
	delete(m.cache.Items, id)
	m.enqueue(Command{Type: "item_delete", UUID: genID(), Args: map[string]any{"id": id}})
	m.unpinIfMatches(id)
	m.deriveAll()
	m.status = "deleted: " + content
}

// unpinIfMatches releases the pin when the pinned task is finished/removed.
func (m *model) unpinIfMatches(id string) {
	if m.pinnedID == id {
		m.pinnedID = ""
	}
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

// toggleLabel adds the label to a task if absent, removes it if present.
func (m *model) toggleLabel(id, label string) {
	it, ok := m.cache.Items[id]
	if !ok {
		return
	}
	has := false
	var next []string
	for _, l := range it.Labels {
		if l == label {
			has = true
			continue
		}
		next = append(next, l)
	}
	if !has {
		next = append(next, label)
	}
	it.Labels = next
	m.cache.Items[id] = it
	m.enqueue(Command{Type: "item_update", UUID: genID(), Args: map[string]any{"id": id, "labels": next}})
	m.deriveAll()
	m.refreshDetail()
	if has {
		m.status = "removed @" + label
	} else {
		m.status = "added @" + label
	}
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
	if !ok || it.sep {
		return Task{}, false
	}
	return it.t, true
}

// readonlyGuard blocks mutating actions on completed tasks (shown read-only in
// the completed view). It returns true and sets a status when t is completed.
func (m *model) readonlyGuard(t Task) bool {
	if t.Done {
		m.status = "✓ completed — read-only"
		return true
	}
	return false
}

// applyView rebuilds the visible list from allTasks, narrowed by the local
// text query (case-insensitive substring over content, project and labels).
func (m *model) applyView() {
	// Pin (focus) mode overrides everything: show only the pinned task.
	if m.pinnedID != "" {
		var items []list.Item
		for _, t := range m.allTasks {
			if t.ID == m.pinnedID {
				items = append(items, taskItem{t: t})
				break
			}
		}
		m.list.SetItems(items)
		return
	}
	if m.onlineView {
		items := make([]list.Item, len(m.onlineResults))
		for i, t := range m.onlineResults {
			items[i] = taskItem{t: t}
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
				items = append(items, taskItem{t: t})
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

	// The completed view is only available inside a single project; elsewhere it
	// always falls back to active-only.
	cv := m.completedView
	if m.projectView == "" {
		cv = 0
	}

	var items []list.Item
	if cv != 2 { // 0 or 1: include active tasks
		for _, t := range matched {
			items = append(items, taskItem{t: t})
		}
	}
	if cv != 0 { // 1 or 2: include completed tasks (read-only)
		completed := m.filteredCompleted(q)
		if cv == 1 && len(items) > 0 && len(completed) > 0 {
			items = append(items, taskItem{sep: true, label: "completed"})
		}
		for _, t := range completed {
			items = append(items, taskItem{t: t})
		}
	}
	m.list.SetItems(items)
}

// filteredCompleted returns completed tasks narrowed by the active project,
// priority and text filters (date filters are skipped — completed tasks rarely
// match "today"-style queries).
func (m *model) filteredCompleted(q string) []Task {
	if m.cache == nil {
		return nil
	}
	var out []Task
	for _, t := range m.cache.CompletedTasks() {
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
		out = append(out, t)
	}
	m.sortTasks(out)
	return out
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
	case sortAdded:
		// Default direction (ascending) puts the most recently added first,
		// matching the R "recently added" view; reverse for oldest first.
		added := func(id string) string {
			if m.cache != nil {
				return m.cache.Items[id].AddedAt
			}
			return ""
		}
		less = func(i, j int) bool { return added(ts[i].ID) > added(ts[j].ID) }
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
	m.completedView = 0 // start each view showing active tasks only
	m.applyView()       // everything is local now
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
			m.syncBg = false
			m.spinFrame = 0
			cmds := []tea.Cmd{syncNowCmd(m.cache.SyncToken, m.queue), m.restartAutoSync(), spinnerTick()}
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

	case completedFetchedMsg:
		if msg.err != nil {
			m.status = "completed: showing local only (" + msg.err.Error() + ")"
			return m, nil
		}
		for _, it := range msg.items {
			if e, ok := m.cache.Items[it.ID]; ok {
				e.Checked = true // keep richer local copy, just mark done
				m.cache.Items[it.ID] = e
			} else {
				m.cache.Items[it.ID] = it
			}
		}
		m.cache.Save()
		m.deriveAll() // applyView re-renders the completed section
		m.status = fmt.Sprintf("completed loaded from Todoist (%d)", len(msg.items))
		return m, nil

	case homeFlashOffMsg:
		m.homeFlash = false
		return m, nil

	case spinnerTickMsg:
		if !m.syncing {
			m.spinFrame = 0
			return m, nil // sync finished — stop animating
		}
		m.spinFrame++
		return m, spinnerTick()

	case autoSyncTickMsg:
		if msg.gen != m.tickGen || m.settings.SyncSeconds <= 0 {
			return m, nil // stale or disabled
		}
		cmds := []tea.Cmd{autoSyncCmd(m.tickGen, m.settings.SyncSeconds)} // reschedule
		if !m.syncing && HasToken() {
			m.syncing = true
			m.syncBg = true
			m.spinFrame = 0
			m.status = "auto-syncing…"
			cmds = append(cmds, syncNowCmd(m.cache.SyncToken, m.queue), spinnerTick())
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
		mapping := map[string]string{}
		if msg.resp != nil {
			mapping = msg.resp.TempIDMapping
		}
		m.cache.Merge(msg.resp)
		m.syncMindLinks(mapping)
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
		case modeCommand:
			return m.updateCommand(msg)
		case modeProjectAdd:
			return m.updateProjectAdd(msg)
		case modeProjectDelete:
			return m.updateProjectDelete(msg)
		case modeIdeaAdd:
			return m.updateIdeaAdd(msg)
		case modeIdeaList:
			return m.updateIdeaList(msg)
		case modeIdeaRename:
			return m.updateIdeaRename(msg)
		case modeMindMap:
			return m.updateMindMap(msg)
		case modeMindEdit:
			return m.updateMindEdit(msg)
		case modeMindHelp:
			// Any key closes the mind-map help and returns to the map.
			m.mode = modeMindMap
			return m, nil
		case modeMindPalette:
			return m.updateMindPalette(msg)
		case modeMindConfirmDelete:
			return m.updateMindConfirmDelete(msg)
		case modeMindConfirmUnbind:
			return m.updateMindConfirmUnbind(msg)
		case modeDeadlinePick:
			return m.updateDeadlinePick(msg)
		case modeTimezone:
			return m.updateTimezone(msg)
		case modePalette:
			return m.updatePalette(msg)
		case modeAbout:
			// Any key closes the about screen.
			m.mode = modeList
			return m, nil
		}
	}

	// default: pass to list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While pinned (focus mode), block view-switching to keep you on one task.
	if m.pinnedID != "" {
		switch msg.String() {
		case ">":
			// Add a comment to the pinned task.
			m.detailID = m.pinnedID
			m.mode = modeCommentAdd
			m.input.EchoMode = textinput.EchoNormal
			m.input.Placeholder = "write a comment…"
			m.input.SetValue("")
			m.input.CursorEnd()
			m.input.Focus()
			return m, textinput.Blink
		case "v":
			// Toggle the comments list on the focus card.
			m.showComments = !m.showComments
			if m.showComments && m.cache != nil {
				m.comments = m.cache.CommentsFor(m.pinnedID)
			}
			return m, nil
		case "a", "A", "p", "P", "o", "f", "t", "T", "W", "m", "M", "d", "D", "R", "Y",
			"/", "?", ",", "b", "h", "1", "2", "3", "4", "5", "6", "7", "0":
			m.status = "📌 pinned — type :unpin (then Enter) to switch tasks"
			return m, nil
		}
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "`":
		// Quick-action palette — search & run any command (VSCode-style).
		m.mode = modePalette
		m.palQuery = ""
		m.palCursor = 0
		return m, nil
	case "~":
		// About screen.
		m.mode = modeAbout
		return m, nil
	case "+":
		m.settings.Light = !m.settings.Light
		m.settings.Save()
		m.applyThemeFromSettings()
		if m.settings.Light {
			m.status = "light theme"
		} else {
			m.status = "dark theme"
		}
		return m, nil
	case ":":
		m.mode = modeCommand
		m.err = ""
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "unpin · q"
		m.input.SetValue("")
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "^":
		// Pin the selected task — focus mode for this session (only this task shows).
		if t, ok := m.selectedTask(); ok {
			m.pinnedID = t.ID
			m.applyView()
			m.status = "📌 pinned: " + t.Content
		}
		return m, nil
	case "i":
		// 💡 Catch an idea (works even while pinned).
		m.mode = modeIdeaAdd
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "What's the idea?"
		m.input.SetValue("")
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "I":
		// 💡 Browse captured ideas.
		m.ideas = LoadIdeas()
		m.ideaCursor = 0
		m.mode = modeIdeaList
		return m, nil
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
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
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
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
			m.completeTask(t.ID, t.Content)
		}
		return m, nil
	case "C":
		// Reopen (un-complete) a highlighted completed task.
		if t, ok := m.selectedTask(); ok {
			if !t.Done {
				m.status = "C reopens a completed task — this one is already active"
				return m, nil
			}
			m.uncompleteTask(t.ID, t.Content)
		}
		return m, nil
	case "o":
		// Ongoing — show all tasks tagged with the configured label.
		return m, m.commit(viewState{filter: "@" + m.settings.OngoingLabel})
	case "f":
		// Follow-up — show all tasks tagged with the configured label.
		return m, m.commit(viewState{filter: "@" + m.settings.FollowupLabel})
	case "u":
		// Up Next — show all tasks tagged with the configured label.
		return m, m.commit(viewState{filter: "@" + m.settings.UpNextLabel})
	case "t":
		// Tasks due today only.
		return m, m.commit(viewState{filter: "today"})
	case "T":
		// Tasks due today and earlier (today + overdue).
		return m, m.commit(viewState{filter: "today | overdue"})
	case "W":
		// Tasks due this week or last week.
		return m, m.commit(viewState{filter: "this week"})
	case "m":
		// Tasks due this month.
		return m, m.commit(viewState{filter: "this month"})
	case "M":
		// Tasks due this month or last month.
		return m, m.commit(viewState{filter: "this+last month"})
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
	case ",":
		// Menu (settings).
		m.mode = modeOptions
		m.optCursor = 0
		m.err = ""
		return m, nil
	case "O":
		// Tag/untag the selected task with the ongoing label.
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
			m.toggleLabel(t.ID, m.settings.OngoingLabel)
		}
		return m, nil
	case "F":
		// Tag/untag the selected task with the follow-up label.
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
			m.toggleLabel(t.ID, m.settings.FollowupLabel)
		}
		return m, nil
	case "U":
		// Tag/untag the selected task with the up-next label.
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
			m.toggleLabel(t.ID, m.settings.UpNextLabel)
		}
		return m, nil
	case "n":
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		return m, cmd
	case "v":
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		return m, cmd
	case "up", "down", "k", "j":
		// Move, then hop over the non-selectable "completed" separator so up/down
		// jumps straight between tasks.
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if it, ok := m.list.SelectedItem().(taskItem); ok && it.sep {
			m.list, cmd = m.list.Update(msg)
		}
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
	case "Y":
		// Completed view only makes sense inside a single project.
		if m.projectView == "" {
			m.status = "open a project first (p) — completed view is per-project"
			return m, nil
		}
		m.completedView = (m.completedView + 1) % 3
		m.applyView()
		switch m.completedView {
		case 1:
			m.status = "showing active + completed (loading from Todoist…)"
		case 2:
			m.status = "showing completed only (loading from Todoist…)"
		default:
			m.status = "showing active tasks"
			return m, nil
		}
		// Pull completed tasks for this project from the server (local cache shows
		// immediately; the server results merge in when they arrive).
		if HasToken() {
			if p, ok := m.projectByName(m.projectView); ok {
				return m, fetchCompletedCmd(p.ID)
			}
		}
		return m, nil
	case "x":
		if t, ok := m.selectedTask(); ok && !m.readonlyGuard(t) {
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
		m.syncBg = false
		m.spinFrame = 0
		m.status = "syncing…"
		return m, tea.Batch(syncNowCmd(m.cache.SyncToken, m.queue), spinnerTick())
	case "b":
		return m, m.goBack()
	case "h", "esc":
		// Home — clear all filters/views (undoable with b), with a brief flash.
		m.homeFlash = true
		cmd := m.commit(viewState{})
		flash := tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return homeFlashOffMsg{} })
		return m, tea.Batch(cmd, flash)
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
	case "7":
		m.setSort(sortAdded)
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
		if m.pickIntent == pickMindTasks {
			m.mode = modeMindMap // back to the map we came from
			return m, nil
		}
		m.mode = modeList
		return m, nil
	case "up", "down", "ctrl+p", "pgup", "pgdown":
		var cmd tea.Cmd
		m.projList, cmd = m.projList.Update(msg)
		return m, cmd
	case "ctrl+n":
		// New project.
		m.mode = modeProjectAdd
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "new project name"
		m.input.SetValue("")
		m.input.CursorEnd()
		m.input.Focus()
		return m, textinput.Blink
	case "ctrl+d":
		// Delete the selected project (asks first).
		if it, ok := m.projList.SelectedItem().(projItem); ok &&
			it.kind != kindSep && it.p.ID != allProjectsID && it.p.ID != "" {
			m.projDelTarget = it.p
			m.mode = modeProjectDelete
		}
		return m, nil
	case "ctrl+e":
		// Archive the selected project, then sync right away.
		if it, ok := m.projList.SelectedItem().(projItem); ok &&
			it.kind != kindSep && it.p.ID != allProjectsID && it.p.ID != "" {
			m.archiveProjectLocal(it.p)
			return m, m.startSync()
		}
		return m, nil
	case "backspace":
		if m.projQuery != "" {
			r := []rune(m.projQuery)
			m.projQuery = string(r[:len(r)-1])
			m.setPickerItems()
			m.projList.Select(0)
		}
		return m, nil
	case "enter":
		if m.pickIntent == pickMindTasks {
			return m.commitMindTasks()
		}
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

// addProjectLocal creates a project optimistically and queues a project_add.
func (m *model) addProjectLocal(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	temp := "tmp-" + genID()
	m.cache.Projects[temp] = apiProject{ID: temp, Name: name}
	m.enqueue(Command{Type: "project_add", UUID: genID(), TempID: temp, Args: map[string]any{"name": name}})
	m.deriveAll()
	m.setPickerItems()
	m.status = "added project: #" + name
}

// commitMindTasks adds every task-flagged mind-map node as a Todoist task under
// the chosen project. If the typed name matches no existing project, it is
// auto-created first. Each node is linked to its new task id.
func (m model) commitMindTasks() (tea.Model, tea.Cmd) {
	q := strings.TrimPrefix(strings.TrimSpace(m.projQuery), "#")
	var target Project
	switch {
	case q != "" && !m.projectExists(q):
		// Typed a name that doesn't exist yet — create it.
		m.addProjectLocal(q)
		if p, ok := m.projectByName("#" + q); ok {
			target = p
		}
	default:
		it, ok := m.projList.SelectedItem().(projItem)
		if ok && it.kind != kindSep && it.p.ID != "" && it.p.ID != allProjectsID {
			target = it.p
		} else if q != "" {
			if p, ok := m.projectByName("#" + q); ok {
				target = p
			}
		}
	}
	if target.ID == "" {
		m.status = "pick or type a project name"
		return m, nil
	}
	m = m.convertMindTasksTo(target)
	m.mode = modeMindMap
	return m, nil
}

// convertMindTasksTo adds the idea's unlinked task nodes to target, links each to
// its new task id, and binds the idea to that one project.
func (m model) convertMindTasksTo(target Project) model {
	tasks := m.ideas[m.mindIdea].collectMindTasks()
	for _, n := range tasks {
		n.TaskID = m.addTask(n.Text, target)
	}
	m.ideas[m.mindIdea].ProjectID = target.ID
	m.ideas[m.mindIdea].ProjectName = target.Name
	m.recents = pushRecentProject(m.recents, target)
	SaveRecentProjects(m.recents)
	SaveIdeas(m.ideas)
	m.status = fmt.Sprintf("✅ added %d task(s) to %s — press s to sync", len(tasks), target.Name)
	return m
}

// projectByID finds a project by its Todoist id.
func (m *model) projectByID(id string) (Project, bool) {
	for _, p := range m.projects {
		if p.ID == id {
			return p, true
		}
	}
	return Project{}, false
}

// syncMindLinks reconciles linked mind-map nodes after a sync: it remaps any
// temporary task ids to their real ids, and marks a node done when its linked
// task has been completed in Todoist.
func (m *model) syncMindLinks(mapping map[string]string) {
	changed := false
	var walk func(ns []*MindNode)
	walk = func(ns []*MindNode) {
		for _, n := range ns {
			if n.TaskID != "" {
				if real, ok := mapping[n.TaskID]; ok && real != "" {
					n.TaskID = real
					changed = true
				}
				if it, ok := m.cache.Items[n.TaskID]; ok && it.Checked != n.Done {
					n.Done = it.Checked
					changed = true
				}
			}
			walk(n.Children)
		}
	}
	for i := range m.ideas {
		// Remap an auto-created project's temp id to its real id once it syncs.
		if real, ok := mapping[m.ideas[i].ProjectID]; ok && real != "" {
			m.ideas[i].ProjectID = real
			changed = true
		}
		walk(m.ideas[i].Children)
	}
	if changed {
		SaveIdeas(m.ideas)
	}
}

// projectExists reports whether a project's name (without the leading #) matches
// the given text case-insensitively.
func (m *model) projectExists(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range m.projects {
		if strings.ToLower(strings.TrimPrefix(p.Name, "#")) == name {
			return true
		}
	}
	return false
}

// archiveProjectLocal archives a project (kept on the server, hidden here).
func (m *model) archiveProjectLocal(p Project) {
	if it, ok := m.cache.Projects[p.ID]; ok {
		it.IsArchived = true
		m.cache.Projects[p.ID] = it
	}
	m.enqueue(Command{Type: "project_archive", UUID: genID(), Args: map[string]any{"id": p.ID}})
	m.recents = removeRecentProject(m.recents, p.ID)
	SaveRecentProjects(m.recents)
	if m.projectView == p.Name { // leave a view filtered to the archived project
		m.projectView = ""
	}
	m.deriveAll()
	m.setPickerItems()
	m.status = "archived project: " + p.Name
}

// deleteProjectLocal removes a project (and its cached tasks) and queues a delete.
func (m *model) deleteProjectLocal(p Project) {
	delete(m.cache.Projects, p.ID)
	for id, it := range m.cache.Items {
		if it.ProjectID == p.ID {
			delete(m.cache.Items, id)
		}
	}
	m.enqueue(Command{Type: "project_delete", UUID: genID(), Args: map[string]any{"id": p.ID}})
	m.recents = removeRecentProject(m.recents, p.ID)
	SaveRecentProjects(m.recents)
	if m.projectView == p.Name {
		m.projectView = ""
	}
	m.deriveAll()
	m.setPickerItems()
	m.status = "deleted project: " + p.Name
}

func (m model) updateDeadlinePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	choose := func(i int) (tea.Model, tea.Cmd) {
		if i < 0 || i >= len(deadlineOptions) {
			return m, nil
		}
		switch deadlineOptions[i].phrase {
		case "custom":
			return m, m.startEdit(efDeadline, dateInputHint()+" · or today/tomorrow/next week", m.detailTask.Deadline)
		case "clear":
			m.setDeadline(m.detailID, "")
			m.mode = modeDetail
			return m, nil
		default:
			m.setDeadline(m.detailID, parseHumanDate(deadlineOptions[i].phrase, todayStr()))
			m.mode = modeDetail
			return m, nil
		}
	}
	switch msg.String() {
	case "esc", "b":
		m.mode = modeDetail
		return m, nil
	case "up", "k":
		if m.dlCursor > 0 {
			m.dlCursor--
		}
		return m, nil
	case "down", "j":
		if m.dlCursor < len(deadlineOptions)-1 {
			m.dlCursor++
		}
		return m, nil
	case "enter":
		return choose(m.dlCursor)
	}
	if r := msg.String(); len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
		return choose(int(r[0] - '1'))
	}
	return m, nil
}

func (m model) updateIdeaAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		m.mode = modeList
		m.input.Blur()
		if text != "" {
			m.ideas = addIdea(m.ideas, text)
			SaveIdeas(m.ideas)
			m.status = "💡 idea saved (" + fmt.Sprintf("%d", len(m.ideas)) + ")"
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateIdeaList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "I", "b":
		m.mode = modeList
		return m, nil
	case "enter", "right", "l":
		// Open the selected idea as a mind map (the idea is the root node).
		if m.ideaCursor >= 0 && m.ideaCursor < len(m.ideas) {
			m.mindIdea = m.ideaCursor
			m.mindCursor = 0
			m.mode = modeMindMap
		}
		return m, nil
	case "R", "e":
		// Rename the selected idea.
		if m.ideaCursor >= 0 && m.ideaCursor < len(m.ideas) {
			m.mode = modeIdeaRename
			m.input.EchoMode = textinput.EchoNormal
			m.input.Placeholder = "rename idea…"
			m.input.SetValue(m.ideas[m.ideaCursor].Text)
			m.input.CursorEnd()
			m.input.Focus()
			return m, textinput.Blink
		}
		return m, nil
	case "up", "k":
		if m.ideaCursor > 0 {
			m.ideaCursor--
		}
		return m, nil
	case "down", "j":
		if m.ideaCursor < len(m.ideas)-1 {
			m.ideaCursor++
		}
		return m, nil
	case "x", "d":
		// Delete the selected idea.
		if m.ideaCursor >= 0 && m.ideaCursor < len(m.ideas) {
			m.ideas = append(m.ideas[:m.ideaCursor], m.ideas[m.ideaCursor+1:]...)
			SaveIdeas(m.ideas)
			if m.ideaCursor >= len(m.ideas) && m.ideaCursor > 0 {
				m.ideaCursor--
			}
		}
		return m, nil
	}
	return m, nil
}

// updateIdeaRename commits a rename of the selected idea. Empty input keeps the
// previous name (at least one character is required).
func (m model) updateIdeaRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeIdeaList
		m.input.Blur()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		m.mode = modeIdeaList
		m.input.Blur()
		if m.ideaCursor >= 0 && m.ideaCursor < len(m.ideas) {
			if text == "" {
				m.status = "name can't be empty — kept the previous one"
			} else {
				m.ideas[m.ideaCursor].Text = text
				SaveIdeas(m.ideas)
				m.status = "idea renamed"
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// mindRow is one visible line of the flattened mind-map tree. node is nil for
// the root (which is the idea itself). parent points to the slice that contains
// node, so siblings can be inserted/removed; prefix is the box-drawing gutter.
type mindRow struct {
	node   *MindNode
	parent *[]*MindNode
	index  int // node's position within *parent (-1 for root)
	depth  int
	prefix string
	last   bool // node is the last among its siblings
	isRoot bool
}

// label returns the text shown for a row.
func (r mindRow) label(idea Idea) string {
	if r.isRoot {
		return idea.Text
	}
	return r.node.Text
}

// mindRows flattens the current idea's mind map into the visible rows, skipping
// the children of collapsed nodes and precomputing each row's connector prefix.
func (m model) mindRows() []mindRow {
	if m.mindIdea < 0 || m.mindIdea >= len(m.ideas) {
		return nil
	}
	idea := &m.ideas[m.mindIdea]
	rows := []mindRow{{isRoot: true, index: -1}}
	var walk func(children *[]*MindNode, depth int, ancestorsLast []bool)
	walk = func(children *[]*MindNode, depth int, ancestorsLast []bool) {
		for i := range *children {
			n := (*children)[i]
			last := i == len(*children)-1
			var b strings.Builder
			for _, al := range ancestorsLast {
				if al {
					b.WriteString("   ")
				} else {
					b.WriteString("│  ")
				}
			}
			if last {
				b.WriteString("└─ ")
			} else {
				b.WriteString("├─ ")
			}
			rows = append(rows, mindRow{
				node: n, parent: children, index: i, depth: depth,
				prefix: b.String(), last: last,
			})
			if !n.Collapsed && len(n.Children) > 0 {
				walk(&n.Children, depth+1, append(append([]bool{}, ancestorsLast...), last))
			}
		}
	}
	walk(&idea.Children, 1, nil)
	return rows
}

// mindIndexOf returns the visible row index of target (0 if not found/visible).
func (m model) mindIndexOf(target *MindNode) int {
	for i, r := range m.mindRows() {
		if r.node == target {
			return i
		}
	}
	return 0
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// moveMindVertical moves the cursor to the nearest node above (down=false) or
// below (down=true) the current one by vertical position. Used when up/down has
// no sibling to move to, so the cursor jumps to the node visually adjacent.
func (m *model) moveMindVertical(cur mindRow, down bool) {
	_, boxes, _, _ := m.buildMindBoxes()
	var curBox *mindBox
	for _, b := range boxes {
		if (cur.isRoot && b.isRoot) || (!cur.isRoot && b.node == cur.node) {
			curBox = b
			break
		}
	}
	if curBox == nil {
		return
	}
	// Candidate picker: nearest by row, ties broken by horizontal closeness.
	pick := func(sameColOnly bool) *mindBox {
		var best *mindBox
		for _, b := range boxes {
			if b == curBox {
				continue
			}
			if down && b.cy <= curBox.cy {
				continue
			}
			if !down && b.cy >= curBox.cy {
				continue
			}
			if sameColOnly && b.x != curBox.x {
				continue // restrict to the same column (same depth)
			}
			if best == nil {
				best = b
				continue
			}
			bd, cd := absInt(b.cy-curBox.cy), absInt(best.cy-curBox.cy)
			if bd < cd || (bd == cd && absInt(b.x-curBox.x) < absInt(best.x-curBox.x)) {
				best = b
			}
		}
		return best
	}
	// Prefer a node in the same column (same depth); fall back to any node so the
	// last node in a column can still step down/up into a neighbouring branch.
	best := pick(true)
	if best == nil {
		best = pick(false)
	}
	if best == nil {
		return
	}
	if best.isRoot {
		m.mindCursor = 0
	} else {
		m.mindCursor = m.mindIndexOf(best.node)
	}
}

func (m model) updateMindMap(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.mindRows()
	if len(rows) == 0 {
		m.mode = modeIdeaList
		return m, nil
	}
	if m.mindCursor < 0 {
		m.mindCursor = 0
	}
	if m.mindCursor >= len(rows) {
		m.mindCursor = len(rows) - 1
	}
	cur := rows[m.mindCursor]

	switch msg.String() {
	case "esc", "q", "b":
		// Back to the idea list (b is "back" everywhere).
		SaveIdeas(m.ideas)
		m.mode = modeIdeaList
		return m, nil
	case "H", "?":
		// Dedicated mind-map keyboard help.
		m.mode = modeMindHelp
		return m, nil
	case "r":
		// Jump back to the root node (left-most / first node).
		m.mindCursor = 0
		return m, nil
	case "R":
		// Rename the root node — i.e. the idea itself (label + stored idea text).
		m.mindCursor = 0
		return m.beginMindEdit(nil, m.ideas[m.mindIdea].Text)
	case "`":
		// Quick-action palette (mirrors the task list's ` palette).
		m.mode = modeMindPalette
		m.palQuery = ""
		m.palCursor = 0
		return m, nil
	case "up", "k":
		// Previous sibling; if there isn't one, the nearest node above.
		if !cur.isRoot && cur.parent != nil && cur.index > 0 {
			m.mindCursor = m.mindIndexOf((*cur.parent)[cur.index-1])
			return m, nil
		}
		m.moveMindVertical(cur, false)
		return m, nil
	case "down", "j":
		// Next sibling; if there isn't one, the nearest node below.
		if !cur.isRoot && cur.parent != nil && cur.index+1 < len(*cur.parent) {
			m.mindCursor = m.mindIndexOf((*cur.parent)[cur.index+1])
			return m, nil
		}
		m.moveMindVertical(cur, true)
		return m, nil
	case "left", "h":
		// Go back to the parent — the node one column to the left. (Folding is
		// on space only.)
		if cur.isRoot {
			return m, nil
		}
		for i := m.mindCursor - 1; i >= 0; i-- {
			if rows[i].depth < cur.depth {
				m.mindCursor = i
				break
			}
		}
		return m, nil
	case "right", "l":
		// Descend to the first child of an expanded branch.
		if cur.isRoot {
			if m.mindCursor < len(rows)-1 {
				m.mindCursor++
			}
			return m, nil
		}
		if len(cur.node.Children) > 0 && !cur.node.Collapsed && m.mindCursor < len(rows)-1 {
			m.mindCursor++
		}
		return m, nil
	case "c", "C", "v", "V":
		// Colour cycling: c/C = outline, v/V = background; uppercase also paints
		// every descendant. The root's colour lives on the idea itself.
		key := msg.String()
		subtree := key == "C" || key == "V"
		outline := key == "c" || key == "C"
		if cur.isRoot {
			idea := &m.ideas[m.mindIdea]
			if outline {
				idea.Color = nextMindColor(idea.Color)
				if subtree {
					setSubtreeOutline(idea.Children, idea.Color)
				}
			} else {
				idea.BG = nextMindColor(idea.BG)
				if subtree {
					setSubtreeBG(idea.Children, idea.BG)
				}
			}
		} else {
			if outline {
				cur.node.Color = nextMindColor(cur.node.Color)
				if subtree {
					setSubtreeOutline(cur.node.Children, cur.node.Color)
				}
			} else {
				cur.node.BG = nextMindColor(cur.node.BG)
				if subtree {
					setSubtreeBG(cur.node.Children, cur.node.BG)
				}
			}
		}
		SaveIdeas(m.ideas)
		return m, nil
	case "tab", "o":
		// Tab adds a child to the selected node.
		n := &MindNode{}
		if cur.isRoot {
			m.ideas[m.mindIdea].Children = append(m.ideas[m.mindIdea].Children, n)
		} else {
			cur.node.Collapsed = false
			cur.node.Children = append(cur.node.Children, n)
		}
		m.mindCursor = m.mindIndexOf(n)
		return m.beginMindEdit(n, "")
	case "enter":
		// Enter adds a sibling after the selected node (a child of the root,
		// which has no siblings).
		n := &MindNode{}
		if cur.isRoot {
			m.ideas[m.mindIdea].Children = append(m.ideas[m.mindIdea].Children, n)
		} else {
			*cur.parent = insertMindNode(*cur.parent, cur.index+1, n)
		}
		m.mindCursor = m.mindIndexOf(n)
		return m.beginMindEdit(n, "")
	case "e", "i", "f2":
		// Edit the selected node's text (i / e / F2).
		if cur.isRoot {
			return m.beginMindEdit(nil, m.ideas[m.mindIdea].Text)
		}
		return m.beginMindEdit(cur.node, cur.node.Text)
	case " ":
		// Toggle collapse on a branch.
		if !cur.isRoot && len(cur.node.Children) > 0 {
			cur.node.Collapsed = !cur.node.Collapsed
			SaveIdeas(m.ideas)
		}
		return m, nil
	case "t":
		// Mark / unmark the selected node as an actionable task.
		if cur.isRoot {
			m.status = "the root is the idea — mark a branch as a task instead"
			return m, nil
		}
		cur.node.IsTask = !cur.node.IsTask
		SaveIdeas(m.ideas)
		if cur.node.IsTask {
			m.status = "✅ marked as task"
		} else {
			m.status = "unmarked task"
		}
		return m, nil
	case "T":
		// Convert task-marked nodes into Todoist tasks. An idea binds to a single
		// project: once bound, T just tops up any new tasks and reminds you to sync.
		idea := &m.ideas[m.mindIdea]
		pending := idea.collectMindTasks() // task nodes not yet linked
		if idea.ProjectID != "" {
			if p, ok := m.projectByID(idea.ProjectID); ok {
				if len(pending) == 0 {
					m.status = "already bound to " + idea.ProjectName + " — press s to sync"
					return m, nil
				}
				m = m.convertMindTasksTo(p)
				m.status = fmt.Sprintf("added %d new task(s) to %s — press s to sync", len(pending), p.Name)
				return m, nil
			}
		}
		if len(pending) == 0 {
			m.status = "no nodes marked as task — press t on nodes first"
			return m, nil
		}
		m.mode = modeProjectPick
		m.pickIntent = pickMindTasks
		m.err = ""
		m.projQuery = ""
		m.setPickerItems()
		m.selectLastProject()
		return m, nil
	case "U":
		// Unbind the idea from its project — ask first.
		if m.ideas[m.mindIdea].ProjectID == "" {
			m.status = "this idea isn't bound to a project"
			return m, nil
		}
		m.mode = modeMindConfirmUnbind
		return m, nil
	case "s":
		// Sync. First top up the bound project with any new task nodes, then flush
		// and pull. Tasks already linked (or deleted in Todoist) are left alone.
		if m.syncing {
			return m, nil
		}
		added := m.topUpBoundIdeas()
		m.syncing = true
		m.syncBg = false
		m.spinFrame = 0
		if added > 0 {
			m.status = fmt.Sprintf("syncing… (+%d new task(s))", added)
		} else {
			m.status = "syncing…"
		}
		return m, tea.Batch(syncNowCmd(m.cache.SyncToken, m.queue), spinnerTick())
	case "x":
		// x only completes/reopens task nodes. On a non-task node it does
		// nothing — use d to delete.
		if cur.isRoot || !cur.node.IsTask {
			m.status = "not a task — press t to mark, or d to delete"
			return m, nil
		}
		return m.toggleMindDone(cur.node), nil
	case "d", "delete":
		// Delete the node and its whole subtree — ask first. The root can't be
		// deleted (it's the idea — rename it with R, or remove it from the list).
		if cur.isRoot {
			m.status = "the root can't be deleted — it's the idea (press R to rename)"
			return m, nil
		}
		m.mindDelTarget = cur.node
		m.mode = modeMindConfirmDelete
		return m, nil
	}
	return m, nil
}

// topUpBoundIdeas creates Todoist tasks for any task-flagged nodes that aren't
// linked yet, in every idea already bound to a project. Nodes already linked
// (including ones whose task was deleted in Todoist) are left untouched, so a
// manually-deleted task is never recreated. Returns the number added.
func (m *model) topUpBoundIdeas() int {
	total := 0
	for i := range m.ideas {
		idea := &m.ideas[i]
		if idea.ProjectID == "" {
			continue
		}
		// Resolve the bound project. The stored id can go stale (e.g. an
		// auto-created project's temp id changes once it syncs), so fall back to
		// matching by name and self-heal the stored id.
		p, ok := m.projectByID(idea.ProjectID)
		if !ok && idea.ProjectName != "" {
			if p, ok = m.projectByName(idea.ProjectName); ok {
				idea.ProjectID = p.ID
			}
		}
		if !ok {
			continue // bound project no longer exists — skip
		}
		for _, n := range idea.collectMindTasks() {
			n.TaskID = m.addTask(n.Text, p)
			total++
		}
	}
	if total > 0 {
		SaveIdeas(m.ideas)
	}
	return total
}

// updateMindConfirmDelete handles the y/n delete confirmation in the mind map.
// Deleting only removes the node from the map — any linked Todoist task is left
// in place.
func (m model) updateMindConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.mindDelTarget != nil {
			removeMindNode(&m.ideas[m.mindIdea].Children, m.mindDelTarget)
			SaveIdeas(m.ideas)
			if n := len(m.mindRows()); m.mindCursor >= n {
				m.mindCursor = n - 1
			}
			if m.mindCursor < 0 {
				m.mindCursor = 0
			}
			m.status = "node deleted"
		}
		m.mindDelTarget = nil
		m.mode = modeMindMap
		return m, nil
	case "n", "esc":
		m.mindDelTarget = nil
		m.mode = modeMindMap
		m.status = "delete cancelled"
		return m, nil
	}
	return m, nil
}

// updateMindConfirmUnbind handles the y/n confirmation for unbinding the idea's
// project (which also unlinks the generated task ids).
func (m model) updateMindConfirmUnbind(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		idea := &m.ideas[m.mindIdea]
		was := idea.ProjectName
		idea.ProjectID = ""
		idea.ProjectName = ""
		unlinkMindTasks(idea.Children)
		SaveIdeas(m.ideas)
		m.mode = modeMindMap
		m.status = "unbound from " + was + " — tasks unlinked"
		return m, nil
	case "n", "esc":
		m.mode = modeMindMap
		m.status = "unbind cancelled"
		return m, nil
	}
	return m, nil
}

// toggleMindDone flips a task node's done state, completing or un-completing its
// linked Todoist task (queued for sync) when one exists.
func (m model) toggleMindDone(n *MindNode) model {
	n.Done = !n.Done
	if n.TaskID != "" {
		if it, ok := m.cache.Items[n.TaskID]; ok {
			it.Checked = n.Done
			m.cache.Items[n.TaskID] = it
		}
		if n.Done {
			m.enqueue(Command{Type: "item_complete", UUID: genID(), Args: map[string]any{"id": n.TaskID}})
		} else {
			m.enqueue(Command{Type: "item_uncomplete", UUID: genID(), Args: map[string]any{"id": n.TaskID}})
		}
		m.deriveAll()
	}
	SaveIdeas(m.ideas)
	if n.Done {
		m.status = "✅ task completed"
	} else {
		m.status = "task reopened"
	}
	return m
}

// beginMindEdit opens the node text editor. node == nil edits the root idea.
func (m model) beginMindEdit(node *MindNode, text string) (tea.Model, tea.Cmd) {
	m.mindEditNode = node
	m.mindEditNew = node != nil && node.Text == "" && len(node.Children) == 0
	m.mode = modeMindEdit
	m.input.EchoMode = textinput.EchoNormal
	m.input.Placeholder = "node text…"
	m.input.SetValue(text)
	m.input.CursorEnd()
	m.input.Focus()
	return m, textinput.Blink
}

func (m model) updateMindEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.commitMindEdit(true)
		m.mode = modeMindMap
		return m, nil
	case "enter":
		m.commitMindEdit(false)
		m.mode = modeMindMap
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// commitMindEdit writes (or discards) the node text being edited. A canceled or
// empty brand-new node is removed so the tree never keeps blank placeholders.
func (m *model) commitMindEdit(cancel bool) {
	text := strings.TrimSpace(m.input.Value())
	node := m.mindEditNode
	m.input.Blur()
	switch {
	case node == nil: // editing the root idea
		if !cancel && text != "" {
			m.ideas[m.mindIdea].Text = text
		} else if !cancel {
			m.status = "name can't be empty — kept the previous one"
		}
	case cancel:
		if m.mindEditNew {
			removeMindNode(&m.ideas[m.mindIdea].Children, node)
		}
	case text == "":
		if m.mindEditNew {
			removeMindNode(&m.ideas[m.mindIdea].Children, node)
		} else {
			m.status = "name can't be empty — kept the previous one"
		}
	default:
		node.Text = text
	}
	m.mindEditNode = nil
	m.mindEditNew = false
	if rows := m.mindRows(); m.mindCursor >= len(rows) {
		m.mindCursor = len(rows) - 1
	}
	if m.mindCursor < 0 {
		m.mindCursor = 0
	}
	SaveIdeas(m.ideas)
}

func (m model) updateProjectAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeProjectPick
		m.input.Blur()
		return m, nil
	case "enter":
		name := strings.TrimPrefix(strings.TrimSpace(m.input.Value()), "#")
		m.mode = modeProjectPick
		m.input.Blur()
		if name != "" {
			m.addProjectLocal(name)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateProjectDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.deleteProjectLocal(m.projDelTarget)
		m.mode = modeProjectPick
		return m, m.startSync()
	default:
		m.mode = modeProjectPick
		return m, nil
	}
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
	case "h":
		// Home — close detail and go to the home view.
		m.mode = modeList
		return m, m.commit(viewState{})
	case "^":
		// Pin the task being viewed → focus mode.
		if m.detailID != "" {
			m.pinnedID = m.detailID
			m.mode = modeList
			m.applyView()
			m.status = "📌 pinned: " + m.detailTask.Content
		}
		return m, nil
	case ":":
		m.mode = modeCommand
		m.input.EchoMode = textinput.EchoNormal
		m.input.Placeholder = "unpin · q"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
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
		m.mode = modeDeadlinePick
		m.dlCursor = 0
		return m, nil
	case "l":
		return m, m.startEdit(efLabels, "comma-separated, e.g. ongoing,follow-up", labelsCSV(m.detailTask.Labels))
	case "e":
		return m, m.startEdit(efContent, "task name", m.detailTask.Content)
	case ">":
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
	// Return to the pinned focus card if commenting from there, else the detail view.
	back := modeDetail
	if m.pinnedID != "" && m.detailID == m.pinnedID {
		back = modeList
	}
	switch msg.String() {
	case "esc":
		m.mode = back
		m.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		m.mode = back
		m.input.Blur()
		if val == "" {
			return m, nil
		}
		m.addCommentLocal(m.detailID, val)
		m.showComments = true // jump to showing comments after adding one
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
			if val == "" {
				m.setDeadline(m.detailID, "")
			} else if d := parseHumanDate(normalizeDateInput(val), todayStr()); d != "" {
				m.setDeadline(m.detailID, d)
			} else {
				m.status = "couldn't read date: " + val
			}
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

func (m model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "enter":
		cmd := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.input.Value()), ":")))
		m.mode = modeList
		m.input.Blur()
		switch cmd {
		case "q", "quit", "wq", "q!", "x":
			// vim-style quit: `:q` then Enter.
			return m, tea.Quit
		case "unpin", "unpin task", "u":
			if m.pinnedID != "" {
				m.pinnedID = ""
				m.applyView()
				m.status = "unpinned — all tasks are back"
			} else {
				m.status = "nothing is pinned"
			}
		case "pin":
			if t, ok := m.selectedTask(); ok {
				m.pinnedID = t.ID
				m.applyView()
				m.status = "📌 pinned: " + t.Content
			}
		case "":
			// no-op
		default:
			m.status = "unknown command: :" + cmd
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
		{"Up Next label", "@" + m.settings.UpNextLabel},
		{"Background auto-sync", sync},
		{"Date format", dateInputHint() + "  (enter to cycle)"},
		{"Timezone", m.settings.Timezone + "  " + tzOffsetLabel(m.settings.Timezone) + "  (enter to choose)"},
	}
}

func (m model) updateOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.optionRows()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "b", "O":
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
		if m.optCursor == 4 {
			// Date format cycles in place (no text entry).
			next := map[string]string{"MDY": "YMD", "YMD": "DMY", "DMY": "MDY"}
			m.settings.DateFormat = next[m.settings.DateFormat]
			if m.settings.DateFormat == "" {
				m.settings.DateFormat = "MDY"
			}
			dateFmt = m.settings.DateFormat
			m.settings.Save()
			m.status = "date format: " + dateInputHint()
			return m, nil
		}
		if m.optCursor == 5 {
			// Timezone opens the searchable picker (not an inline text edit).
			if len(m.tzAll) == 0 {
				m.tzAll = availableTimezones()
			}
			m.tzQuery = ""
			m.tzCursor = tzIndexOf(m.tzAll, m.settings.Timezone)
			m.mode = modeTimezone
			return m, nil
		}
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
			m.input.Placeholder = "upnext"
			m.input.SetValue(m.settings.UpNextLabel)
		case 3:
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
			label := strings.TrimPrefix(val, "@")
			if label == "" {
				label = "upnext"
			}
			m.settings.UpNextLabel = label
		case 3:
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
	if m.pinnedID != "" {
		scope = "  📌 pinned"
	} else if m.onlineView {
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
	if m.projectView != "" && m.completedView == 1 {
		scope += lipgloss.NewStyle().Foreground(dueColor).Render("   ✓ +completed")
	} else if m.projectView != "" && m.completedView == 2 {
		scope += lipgloss.NewStyle().Foreground(dueColor).Render("   ✓ completed only")
	}
	left := lipgloss.JoinHorizontal(lipgloss.Center, title, statusStyle.Render(scope))
	ver := lipgloss.NewStyle().Foreground(dimColor).Render("todo-ui " + version + " ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(ver)
	header := left
	if gap > 1 {
		header = lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), ver)
	}
	// While a sync runs, show a cyan indeterminate progress bar under the title
	// bar — visible in every view (task list, mind map, detail, …).
	if m.syncing {
		header = lipgloss.JoinVertical(lipgloss.Left, header, m.syncBar())
	}

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

	if m.mode == modeTimezone {
		return m.timezoneView(header)
	}

	if m.mode == modePalette {
		return m.paletteView(header)
	}

	if m.mode == modeAbout {
		return m.aboutView()
	}

	if m.mode == modeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.helpView())
	}

	if m.mode == modeDetail || m.mode == modeDetailEdit || m.mode == modeCommentAdd || m.mode == modeDeadlinePick {
		if b := m.pinBanner(); b != "" {
			return lipgloss.JoinVertical(lipgloss.Left, header, b, m.detailView())
		}
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
			dim.Render("This signs todo-ui out and wipes its local state:"),
			dim.Render("  • your saved Todoist API token"),
			dim.Render("  • the offline cache (tasks, projects, comments)"),
			dim.Render("  • any changes not yet synced"),
			dim.Render("You'll be asked for your token again to reconnect."),
		}
		if n := len(m.queue); n > 0 {
			rows = append(rows, "", lipgloss.NewStyle().Foreground(warnColor).
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
				pc = textColor
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

	// Idea catcher overlays everything (even a pinned task).
	if m.mode == modeIdeaAdd || m.mode == modeIdeaList || m.mode == modeIdeaRename {
		return m.ideaView(header)
	}

	// Mind-map editor takes over the whole body.
	if m.mode == modeMindMap || m.mode == modeMindEdit {
		return m.mindMapView(header)
	}
	if m.mode == modeMindHelp {
		return m.mindHelpView(header)
	}
	if m.mode == modeMindPalette {
		return m.mindPaletteView(header)
	}
	if m.mode == modeMindConfirmDelete {
		return m.mindConfirmDeleteView(header)
	}
	if m.mode == modeMindConfirmUnbind {
		return m.mindConfirmUnbindView(header)
	}

	// Pinned focus mode: a centered card instead of the list.
	if m.pinnedID != "" && (m.mode == modeList || m.mode == modeCommand || m.mode == modeCommentAdd) {
		return m.pinnedFocusView(header)
	}

	var body string
	switch m.mode {
	case modeProjectPick, modeProjectAdd, modeProjectDelete:
		accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
		prompt := "Add to which project?"
		switch m.pickIntent {
		case pickView:
			prompt = "View which project?"
		case pickMindTasks:
			n := len(m.ideas[m.mindIdea].collectMindTasks())
			prompt = fmt.Sprintf("Add %d marked task(s) to which project? (type a new name to create)", n)
		}
		line := accent.Render(prompt)
		if m.projQuery != "" {
			line += "  " + lipgloss.NewStyle().Foreground(dueColor).Render("filter: "+m.projQuery+"▌")
		}
		var bottom string
		switch m.mode {
		case modeProjectAdd:
			bottom = promptBox.Render(accent.Render("New project  #") + m.input.View())
		case modeProjectDelete:
			bottom = accent.Render("Delete "+m.projDelTarget.Name+" and ALL its tasks?") +
				lipgloss.NewStyle().Foreground(subColor).Render("   y delete · n cancel")
		default:
			bottom = helpStyle.Render("type to filter · ↑/↓ move · enter select · ctrl+n new · ctrl+e archive · ctrl+d delete · esc")
		}
		picker := lipgloss.JoinVertical(lipgloss.Left, line, m.projList.View(), bottom)
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
	case modeCommand:
		label := lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(": ")
		body = promptBox.Render(label + m.input.View() + lipgloss.NewStyle().Foreground(subColor).Render("   (try: unpin · q)"))
	}

	footer := m.footer()
	banner := m.pinBanner()

	// Assemble: header, optional pin banner, optional prompt body, list, footer.
	parts := []string{header}
	used := lipgloss.Height(header) + lipgloss.Height(footer)
	if banner != "" {
		parts = append(parts, banner)
		used += lipgloss.Height(banner)
	}
	if body != "" {
		parts = append(parts, body)
		used += lipgloss.Height(body)
	}
	h := m.height - used
	if h < 3 {
		h = 3
	}
	m.list.SetHeight(h)
	parts = append(parts, m.list.View(), footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ideaView renders the 💡 idea catcher / idea list as a centered yellow card.
func (m model) ideaView(header string) string {
	yellow := lipgloss.NewStyle().Foreground(dueColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	body := lipgloss.NewStyle().Foreground(textColor)

	cardW := m.width - 8
	if cardW > 76 {
		cardW = 76
	}
	if cardW < 30 {
		cardW = 30
	}
	innerW := cardW - 6

	var rows []string
	if m.mode == modeIdeaAdd || m.mode == modeIdeaRename {
		title := "💡  Catch an idea"
		if m.mode == modeIdeaRename {
			title = "💡  Rename idea"
		}
		rows = append(rows,
			yellow.Render(title),
			"",
			dim.Render("Saved locally — not sent to Todoist."),
			"",
			yellow.Render("› ")+m.input.View(),
			"",
			dim.Render("enter save · esc cancel"),
		)
	} else { // modeIdeaList
		rows = append(rows, yellow.Render(fmt.Sprintf("💡  Ideas (%d)", len(m.ideas))), "")
		if len(m.ideas) == 0 {
			rows = append(rows, dim.Render("No ideas yet — press i to catch one."))
		} else {
			for i, idea := range m.ideas {
				cur := "  "
				txtStyle := body
				if i == m.ideaCursor {
					cur = yellow.Render("▸ ")
					txtStyle = lipgloss.NewStyle().Foreground(brightColor).Bold(true)
				}
				when := dim.Render(shortTime(idea.At))
				if n := idea.countNodes(); n > 0 {
					when += "  " + dim.Render(fmt.Sprintf("🗺 %d", n))
				}
				text := txtStyle.Width(innerW).Render(strings.ReplaceAll(strings.TrimSpace(idea.Text), "\n", " "))
				rows = append(rows, cur+when, "  "+text, "")
			}
			rows = append(rows, dim.Render("j/k move · enter open map · R rename · x delete · b/esc back"))
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dueColor).
		Padding(1, 3).
		Width(cardW).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	bodyH := m.height - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, header,
		lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card))
}

// pinnedTask returns the currently pinned task from the cache.
func (m model) pinnedTask() (Task, bool) {
	for _, t := range m.allTasks {
		if t.ID == m.pinnedID {
			return t, true
		}
	}
	return Task{}, false
}

// pinnedFocusView is the full-screen, centered "focus on one task" card.
func (m model) pinnedFocusView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)

	cardW := m.width - 8
	if cardW > 72 {
		cardW = 72
	}
	if cardW < 24 {
		cardW = 24
	}

	// Big-ish title: spaced letters read larger in a terminal.
	pinTitle := accent.Render("📌  P I N N E D")

	var rows []string
	rows = append(rows, pinTitle, "")

	if t, ok := m.pinnedTask(); ok {
		pc := prioColors[t.Priority]
		if pc == "" {
			pc = prioColors["p4"]
		}
		badge := lipgloss.NewStyle().Background(pc).Foreground(brightColor).Bold(true).Render(" " + t.Priority + " ")
		content := lipgloss.NewStyle().Foreground(brightColor).Bold(true).Width(cardW - 8).Align(lipgloss.Center).Render(t.Content)
		rows = append(rows, badge, "", content, "")

		var meta []string
		if t.Project != "" {
			meta = append(meta, lipgloss.NewStyle().Foreground(projectColor).Render(t.Project))
		}
		if t.DueDate != "" {
			meta = append(meta, lipgloss.NewStyle().Foreground(dueColor).Render("due "+fmtDate(t.DueDate)))
		}
		if t.Deadline != "" {
			meta = append(meta, lipgloss.NewStyle().Foreground(deadlineColor).Render("⚑ "+fmtDate(t.Deadline)))
		}
		if t.Labels != "" {
			meta = append(meta, lipgloss.NewStyle().Foreground(labelColor).Render(t.Labels))
		}
		if len(meta) > 0 {
			rows = append(rows, dim.Render(strings.Join(meta, "   ·   ")))
		}
	} else {
		rows = append(rows, dim.Render("This task isn't loaded right now."), dim.Render("Press s to sync, or :unpin to release."))
	}

	rows = append(rows, "", strings.Repeat("─", cardW-8))

	// Comments section (when shown).
	if m.showComments {
		cs := m.comments
		if m.cache != nil {
			cs = m.cache.CommentsFor(m.pinnedID)
		}
		head := lipgloss.NewStyle().Foreground(projectColor).Bold(true).Render(fmt.Sprintf("Comments (%d)", len(cs)))
		rows = append(rows, "", head)
		if len(cs) == 0 {
			rows = append(rows, dim.Render("(none yet — press m to add one)"))
		} else {
			for _, c := range cs {
				when := lipgloss.NewStyle().Foreground(subColor).Render(shortTime(c.PostedAt))
				body := lipgloss.NewStyle().Foreground(textColor).Width(cardW - 8).Align(lipgloss.Center).
					Render(strings.ReplaceAll(strings.TrimSpace(c.Content), "\n", " "))
				rows = append(rows, when, body)
			}
		}
		rows = append(rows, "")
	}

	switch m.mode {
	case modeCommand:
		rows = append(rows, "", lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(": ")+m.input.View())
		rows = append(rows, dim.Render("type unpin, then Enter"))
	case modeCommentAdd:
		rows = append(rows, "", lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("New comment  ")+m.input.View())
		rows = append(rows, dim.Render("enter to post · esc cancel"))
	default:
		cToggle := "v show comments"
		if m.showComments {
			cToggle = "v hide comments"
		}
		rows = append(rows, "", dim.Render("type ")+accent.Render(":unpin")+dim.Render(" then Enter to release"))
		rows = append(rows, accent.Render(">")+dim.Render(" add comment   ")+accent.Render(cToggle)+dim.Render("   enter open · c done · s sync"))
	}

	inner := lipgloss.JoinVertical(lipgloss.Center, rows...)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(brandRed).
		Padding(1, 3).
		Width(cardW).
		Align(lipgloss.Center).
		Render(inner)

	bodyH := m.height - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	centered := lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card)
	return lipgloss.JoinVertical(lipgloss.Left, header, centered)
}

// pinBanner is the prominent "you are pinned" notice with the unpin instruction.
func (m model) pinBanner() string {
	if m.pinnedID == "" {
		return ""
	}
	pin := lipgloss.NewStyle().Background(brandRed).Foreground(brightColor).Bold(true).Render(" 📌 PINNED ")
	tip := lipgloss.NewStyle().Foreground(dueColor).Render("focusing on one task")
	how := lipgloss.NewStyle().Foreground(subColor).Render("— type ") +
		lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render(":unpin") +
		lipgloss.NewStyle().Foreground(subColor).Render(" then Enter to release")
	return " " + pin + "  " + tip + " " + how
}

// optionsView renders the settings page.
func (m model) optionsView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	val := lipgloss.NewStyle().Foreground(projectColor)
	rows := m.optionRows()

	lines := []string{"", "  " + accent.Render("Menu"), ""}
	for i, r := range rows {
		cur := "   "
		name := dim.Render(fmt.Sprintf("%-22s", r.label))
		if i == m.optCursor && m.mode == modeOptions {
			cur = accent.Render(" ▸ ")
			name = lipgloss.NewStyle().Foreground(brightColor).Bold(true).Render(fmt.Sprintf("%-22s", r.label))
		}
		lines = append(lines, cur+name+val.Render(r.value))
	}
	lines = append(lines, "")

	if m.mode == modeOptionsEdit {
		titles := []string{"Ongoing label  @", "Follow-up label  @", "Up Next label  @", "Auto-sync seconds (0 = off)  "}
		box := promptBox.Render(accent.Render(titles[m.optCursor]) + m.input.View())
		lines = append(lines, "  "+box, "", helpStyle.Render("  enter save · esc cancel"))
	} else {
		lines = append(lines, dim.Render("  The ongoing label is what the o key filters on."))
		lines = append(lines, dim.Render("  Auto-sync pushes queued changes & pulls on a timer."))
		lines = append(lines, "", helpStyle.Render("  ↑/↓ move · enter edit · b/esc/, close"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
}

// tzFiltered returns the zones matching the current type-to-filter query.
func (m model) tzFiltered() []string {
	q := strings.ToLower(strings.TrimSpace(m.tzQuery))
	if q == "" {
		return m.tzAll
	}
	var out []string
	for _, z := range m.tzAll {
		if strings.Contains(strings.ToLower(z), q) {
			out = append(out, z)
		}
	}
	return out
}

// updateTimezone drives the searchable IANA timezone picker.
func (m model) updateTimezone(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matches := m.tzFiltered()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeOptions
		return m, nil
	case "enter":
		if len(matches) > 0 {
			if m.tzCursor >= len(matches) {
				m.tzCursor = len(matches) - 1
			}
			name := matches[m.tzCursor]
			m.settings.Timezone = name
			applyTimezone(name)
			m.settings.Save()
			m.deriveAll() // "today" may have shifted — refresh derived views
			m.status = "timezone: " + name + "  " + tzOffsetLabel(name)
		}
		m.mode = modeOptions
		return m, nil
	case "up":
		if m.tzCursor > 0 {
			m.tzCursor--
		}
		return m, nil
	case "down":
		if m.tzCursor < len(matches)-1 {
			m.tzCursor++
		}
		return m, nil
	case "backspace":
		if len(m.tzQuery) > 0 {
			m.tzQuery = m.tzQuery[:len(m.tzQuery)-1]
			m.tzCursor = 0
		}
		return m, nil
	default:
		// Any printable key extends the search (j/k are filter text, not nav).
		if len(msg.Runes) > 0 {
			m.tzQuery += string(msg.Runes)
			m.tzCursor = 0
		}
		return m, nil
	}
}

// timezoneView renders the searchable timezone picker.
func (m model) timezoneView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	val := lipgloss.NewStyle().Foreground(projectColor)
	matches := m.tzFiltered()

	lines := []string{"", "  " + accent.Render("Timezone") + dim.Render("   search: ") + m.tzQuery + "▏", ""}

	const win = 12
	start := 0
	if m.tzCursor >= win {
		start = m.tzCursor - win + 1
	}
	end := start + win
	if end > len(matches) {
		end = len(matches)
	}
	if len(matches) == 0 {
		lines = append(lines, "  "+dim.Render("no match — try a city or region (e.g. Manila, Tokyo, London)"))
	}
	for i := start; i < end; i++ {
		z := matches[i]
		cur := "   "
		name := dim.Render(fmt.Sprintf("%-32s", z))
		if i == m.tzCursor {
			cur = accent.Render(" ▸ ")
			name = lipgloss.NewStyle().Foreground(brightColor).Bold(true).Render(fmt.Sprintf("%-32s", z))
		}
		lines = append(lines, cur+name+val.Render(tzOffsetLabel(z)))
	}
	lines = append(lines, "")
	if len(matches) > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("  %d zones · showing %d–%d", len(matches), start+1, end)))
	}
	lines = append(lines, "", helpStyle.Render("  type to filter · ↑/↓ move · enter select · esc cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
}

// paletteAction is one entry in the ` quick-action palette: a single-key
// command (handled by updateList) and a short human label.
type paletteAction struct{ key, label string }

// paletteActions are the runnable commands shown in the quick-action palette,
// in a sensible default order. Every key is a single rune updateList handles.
var paletteActions = []paletteAction{
	{"a", "Add a task (choose project)"},
	{"A", "Add a task to the recent project"},
	{"c", "Complete the selected task"},
	{"x", "Delete the selected task"},
	{"/", "Search tasks"},
	{"?", "Online search (Todoist filter syntax)"},
	{"p", "View by project"},
	{"P", "Filter by priority"},
	{"o", "Ongoing view"},
	{"f", "Follow-up view"},
	{"u", "Up Next view"},
	{"t", "Due today"},
	{"T", "Due today or earlier"},
	{"W", "Due this or last week"},
	{"m", "Due this month"},
	{"M", "Due this or last month"},
	{"d", "Deadline today"},
	{"D", "Deadline today or earlier"},
	{"R", "Recently added"},
	{"Y", "Show completed (cycle: active · both · completed only)"},
	{"C", "Reopen the selected completed task"},
	{"O", "Tag selected: ongoing"},
	{"F", "Tag selected: follow-up"},
	{"U", "Tag selected: up next"},
	{"^", "Pin selected (focus mode)"},
	{"i", "Catch an idea"},
	{"I", "Browse captured ideas / open mind map"},
	{"+", "Change theme (light / dark)"},
	{",", "Menu / settings"},
	{"s", "Sync now"},
	{"r", "Refresh from cache"},
	{"H", "Help"},
	{"~", "About"},
	{"X", "Clear data"},
	{"b", "Back (previous view)"},
	{"h", "Home (clear filters & views)"},
	{"1", "Sort by priority"},
	{"2", "Sort by due date"},
	{"3", "Sort by deadline"},
	{"4", "Sort by project"},
	{"5", "Sort by name"},
	{"6", "Sort by labels"},
	{"7", "Sort by date added (newest first)"},
	{"0", "Sort: default Todoist order"},
}

// palFiltered returns the actions matching the current type-to-filter query.
func (m model) palFiltered() []paletteAction {
	q := strings.ToLower(strings.TrimSpace(m.palQuery))
	if q == "" {
		return paletteActions
	}
	var out []paletteAction
	for _, a := range paletteActions {
		if strings.Contains(strings.ToLower(a.label), q) || strings.ToLower(a.key) == q {
			out = append(out, a)
		}
	}
	return out
}

// updatePalette drives the ` quick-action palette; enter runs the selected
// command by re-dispatching its key through updateList.
func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matches := m.palFiltered()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		m.mode = modeList
		if len(matches) == 0 {
			return m, nil
		}
		if m.palCursor >= len(matches) {
			m.palCursor = len(matches) - 1
		}
		act := matches[m.palCursor]
		return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(act.key)})
	case "up":
		if m.palCursor > 0 {
			m.palCursor--
		}
		return m, nil
	case "down":
		if m.palCursor < len(matches)-1 {
			m.palCursor++
		}
		return m, nil
	case "backspace":
		if len(m.palQuery) > 0 {
			m.palQuery = m.palQuery[:len(m.palQuery)-1]
			m.palCursor = 0
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.palQuery += string(msg.Runes)
			m.palCursor = 0
		}
		return m, nil
	}
}

// paletteView renders the ` quick-action palette.
func (m model) paletteView(header string) string {
	accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	keyStyle := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	matches := m.palFiltered()

	lines := []string{"", "  " + accent.Render("Quick action") + dim.Render("   search: ") + m.palQuery + "▏", ""}

	const win = 8
	start := 0
	if m.palCursor >= win {
		start = m.palCursor - win + 1
	}
	end := start + win
	if end > len(matches) {
		end = len(matches)
	}
	if len(matches) == 0 {
		lines = append(lines, "  "+dim.Render("no matching action — try 'due', 'tag', 'sort'…"))
	}
	for i := start; i < end; i++ {
		a := matches[i]
		cur := "   "
		key := keyStyle.Render(fmt.Sprintf("%3s", a.key))
		label := dim.Render(a.label)
		if i == m.palCursor {
			cur = accent.Render(" ▸ ")
			label = lipgloss.NewStyle().Foreground(brightColor).Bold(true).Render(a.label)
		}
		lines = append(lines, cur+key+"   "+label)
	}
	lines = append(lines, "")
	if len(matches) > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("  %d action(s) · showing %d–%d", len(matches), start+1, end)))
	}
	lines = append(lines, "", helpStyle.Render("  type to filter · ↑/↓ move · enter run · esc cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
}

// aboutGlyphs are 5-row block letters used to draw the TODO-UI banner.
var aboutGlyphs = map[rune][5]string{
	'T': {"███████", "   █   ", "   █   ", "   █   ", "   █   "},
	'O': {" █████ ", "██   ██", "██   ██", "██   ██", " █████ "},
	'D': {"██████ ", "██   ██", "██   ██", "██   ██", "██████ "},
	'U': {"██   ██", "██   ██", "██   ██", "██   ██", " █████ "},
	'I': {"███", " █ ", " █ ", " █ ", "███"},
	'-': {"     ", "     ", "█████", "     ", "     "},
}

// aboutBanner assembles the big "TODO-UI" block-letter banner, row by row.
func aboutBanner() []string {
	word := []rune("TODO-UI")
	rows := make([]string, 5)
	for r := 0; r < 5; r++ {
		parts := make([]string, 0, len(word))
		for _, ch := range word {
			g := aboutGlyphs[ch]
			parts = append(parts, g[r])
		}
		rows[r] = strings.Join(parts, "  ")
	}
	return rows
}

// aboutView renders the ~ about screen: a big centered logo + contributors.
func (m model) aboutView() string {
	brand := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)
	val := lipgloss.NewStyle().Foreground(projectColor)
	bright := lipgloss.NewStyle().Foreground(brightColor).Bold(true)

	var b []string
	b = append(b, "")
	for _, row := range aboutBanner() {
		b = append(b, brand.Render(row))
	}
	b = append(b,
		"",
		dim.Render("a fast, keyboard-driven Todoist client for the terminal"),
		"",
		val.Render("todo-ui "+version),
		"",
		dim.Render("──────────────  contributors  ──────────────"),
		bright.Render("Carlo C."),
		"",
		dim.Render("press any key to close"),
	)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(brandRed).
		Padding(1, 5).
		Render(lipgloss.JoinVertical(lipgloss.Center, b...))
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
	}
	return card
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
	title := lipgloss.NewStyle().Foreground(brightColor).Bold(true).Render(t.Content)

	lines := []string{
		"",
		"  " + title,
		"",
		field("Priority", t.Priority, lipgloss.NewStyle().Foreground(pc).Bold(true)),
		field("Due", fmtDate(t.DueDate), lipgloss.NewStyle().Foreground(dueColor)),
		field("Deadline", fmtDate(t.Deadline), lipgloss.NewStyle().Foreground(deadlineColor)),
		field("Project", t.Project, lipgloss.NewStyle().Foreground(projectColor)),
		field("Labels", t.Labels, lipgloss.NewStyle().Foreground(labelColor)),
		field("ID", t.ID, label),
		"",
	}

	// Comments section.
	head := lipgloss.NewStyle().Foreground(projectColor).Bold(true)
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
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(brandRed).Render("• ")+when+"  "+lipgloss.NewStyle().Foreground(textColor).Render(body))
		}
	}
	lines = append(lines, "")

	if m.mode == modeDeadlinePick {
		accent := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
		lines = append(lines, accent.Render("  Set deadline"), "")
		for i, o := range deadlineOptions {
			cur := "    "
			st := lipgloss.NewStyle().Foreground(textColor)
			if i == m.dlCursor {
				cur = "  " + accent.Render("▸ ")
				st = lipgloss.NewStyle().Foreground(brightColor).Bold(true)
			}
			num := lipgloss.NewStyle().Foreground(subColor).Render(fmt.Sprintf("%d ", i+1))
			lines = append(lines, cur+num+st.Render(o.label))
		}
		lines = append(lines, "", helpStyle.Render("  ↑/↓ or 1-8 · enter select · esc cancel"))
	} else if m.mode == modeDetailEdit {
		titles := map[editField]string{
			efDate:     "Set due date",
			efDeadline: "Set deadline (" + dateInputHint() + ")",
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
			key.Render(">") + label.Render(" comment  ") +
			key.Render("^") + label.Render(" pin  ") +
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
	head := lipgloss.NewStyle().Foreground(projectColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(subColor)

	row := func(k, desc string) string {
		return "  " + key.Render(fmt.Sprintf("%-12s", k)) + dim.Render(desc)
	}

	return []string{
		"",
		head.Render("  Navigation"),
		row("`", "Quick action — search & run any command"),
		row("~", "About"),
		row("↑/↓ j/k", "Move selection"),
		row("n / v", "Next page / previous page (also pgdn/pgup)"),
		row("b", "Back — return to the previous view (like a browser)"),
		row("h / esc", "Home — clear all filters & views, back to all tasks"),
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
		row(",", "Menu — labels, background-sync interval, date format & timezone"),
		row("X", "Clear data — remove token, cache & queue (asks first)"),
		"",
		head.Render("  In the task view"),
		row("1-4", "Set priority (p1–p4)"),
		row("t", "Set due date    D  set deadline    l  set labels    e  edit name"),
		row(">", "Add a comment (view existing comments above)"),
		row("c", "Complete    b/esc  back to the list"),
		"",
		head.Render("  Tagging"),
		row("O", "Tag/untag the selected task as ongoing"),
		row("F", "Tag/untag the selected task as follow-up"),
		row("U", "Tag/untag the selected task as up next"),
		"",
		head.Render("  Ideas & focus"),
		row("i", "💡 Catch an idea (saved locally; works even while pinned)"),
		row("I", "💡 Browse captured ideas (x delete · esc close)"),
		row("", "   enter on an idea → 🗺 mind map (press H inside for its own help)"),
		row("", "   map: tab child · enter sibling · ←→/hl move · t task · T→project · x done · c/C·b/B colour"),
		row("^", "Pin — focus on one task; only it shows (this session)"),
		row("", "   while pinned: > add comment · v show/hide comments"),
		row(":unpin", "Release the pin (type : then unpin, Enter)"),
		row("+", "Toggle light / dark theme"),
		"",
		head.Render("  Views & filters"),
		row("p", "View by project (pick; ctrl+n new · ctrl+e archive · ctrl+d delete)"),
		row("P", "Filter by priority (pick p1–p4 from the menu)"),
		row("o", "Ongoing — tasks with your ongoing label (set in Menu)"),
		row("f", "Follow-up — tasks with your follow-up label (set in Menu)"),
		row("u", "Up Next — tasks with your up-next label (set in Menu)"),
		row("t", "Due today (only)"),
		row("T", "Due today or earlier (today + overdue)"),
		row("W", "Due this week or last week"),
		row("m", "Due this month"),
		row("M", "Due this month or last month"),
		row("d", "Deadline is today"),
		row("D", "Deadline is today or earlier"),
		row("R", "Recently added — the last 10 tasks you created"),
		row("Y", "Show completed — cycle active · active+completed · completed only (read-only)"),
		row("C", "Reopen the highlighted completed task (un-complete)"),
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
		row("7", "Date added (newest first; press again for oldest)"),
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
	hint := helpStyle.Render(fmt.Sprintf("  j/k ↑/↓ scroll · %s · + change theme · any other key closes", pos))

	return lipgloss.JoinVertical(lipgloss.Left, append(window, hint)...)
}

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "+":
		// Toggle theme without leaving help.
		m.settings.Light = !m.settings.Light
		m.settings.Save()
		m.applyThemeFromSettings()
		return m, nil
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
		badges = append(badges, lipgloss.NewStyle().Foreground(warnColor).Render(fmt.Sprintf("●%d unsynced", n)))
	}
	if m.online {
		badges = append(badges, lipgloss.NewStyle().Foreground(labelColor).Render("online"))
	}
	// Always-visible hint for the quick-action palette.
	badges = append(badges, lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("`")+
		lipgloss.NewStyle().Foreground(subColor).Render(" quick action"))
	// Always-visible hint for the theme toggle.
	badges = append(badges, lipgloss.NewStyle().Foreground(brandRed).Bold(true).Render("+")+
		lipgloss.NewStyle().Foreground(subColor).Render(" change theme"))
	statusLine := statusStyle.Render(st)
	if len(badges) > 0 {
		statusLine = statusStyle.Render(st+"  ") + strings.Join(badges, statusStyle.Render(" · "))
	}
	if m.err != "" {
		statusLine = errStyle.Render("⚠ " + m.err)
	}

	// "H help" and "h home" first and accented so they're always visible even if clipped.
	accentKey := lipgloss.NewStyle().Foreground(brandRed).Bold(true)
	keys := "q quit · a add · enter view · c done · x del · ^ pin · i idea · / find · p project · O/F tag · , menu · s sync"
	homeHint := accentKey.Render("h home/clear")
	if m.homeFlash {
		homeHint = lipgloss.NewStyle().Background(brandRed).Foreground(brightColor).Bold(true).Render(" h home/clear ")
	}
	right := accentKey.Render("H help") + helpStyle.Render(" · ") + homeHint +
		helpStyle.Render(" · "+keys)
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
			fmt.Println("todo-ui", version)
			return
		case "--help", "-h", "help":
			fmt.Println("todo-ui — a terminal UI for Todoist (Sync API, offline-first)")
			fmt.Println("Usage: todo-ui            start the UI")
			fmt.Println("       todo-ui sync       flush queued changes + pull, headless")
			fmt.Println("       todo-ui --version  print version")
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
