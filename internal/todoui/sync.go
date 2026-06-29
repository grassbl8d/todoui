package todoui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// nowStamp is an RFC3339 timestamp for optimistic added_at/posted_at values.
func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

// genID returns a random hex id (used for command uuids and optimistic temp ids).
func genID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// quickParsed is the result of parsing a quick-add string.
type quickParsed struct {
	Content   string
	Labels    []string
	Priority  int    // API priority 1..4 (4 = highest); 1 if unset
	DueString string // natural-language due, parsed server-side on sync
}

// dueKeywords are words that, on their own, clearly begin a date phrase.
var dueKeywords = map[string]bool{
	"today": true, "tomorrow": true, "tom": true, "tonight": true, "tonite": true,
	"yesterday": true,
	"mon":       true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true,
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
	"friday": true, "saturday": true, "sunday": true,
	"jan": true, "feb": true, "mar": true, "apr": true, "may": true, "jun": true,
	"jul": true, "aug": true, "sep": true, "oct": true, "nov": true, "dec": true,
}

// duePrefixKeywords are qualifiers that begin a date phrase only when the word
// that follows is itself date-like. On their own they're ordinary English
// ("work on features", "interested in cooking", "the next big thing"), so we
// require a lookahead before treating them as the start of a due string.
var duePrefixKeywords = map[string]bool{
	"on": true, "in": true, "next": true, "every": true,
}

// dueUnitWords are the time-unit words that can follow a prefix keyword, e.g.
// "next week", "in 3 days", "every month".
var dueUnitWords = map[string]bool{
	"day": true, "days": true, "week": true, "weeks": true, "weekend": true,
	"month": true, "months": true, "year": true, "years": true,
	"hour": true, "hours": true, "min": true, "mins": true,
	"minute": true, "minutes": true,
}

// parseQuickAdd extracts @labels, pN priority and a trailing date phrase from a
// quick-add string. The project comes from the picker, so #tokens are ignored.
func parseQuickAdd(text string) quickParsed {
	out := quickParsed{Priority: 1}
	fields := strings.Fields(text)
	var contentWords []string
	dueStart := -1
	for i, f := range fields {
		low := strings.ToLower(f)
		switch {
		case strings.HasPrefix(f, "@") && len(f) > 1:
			out.Labels = append(out.Labels, strings.TrimPrefix(f, "@"))
		case len(low) == 2 && low[0] == 'p' && low[1] >= '1' && low[1] <= '4':
			// display pN → API priority (5-N)
			out.Priority = 5 - int(low[1]-'0')
		case strings.HasPrefix(f, "#"):
			// project comes from the picker; drop the token
		case dueStart < 0 && startsDatePhrase(fields, i):
			dueStart = i
		default:
			if dueStart < 0 {
				contentWords = append(contentWords, f)
			}
		}
	}
	if dueStart >= 0 {
		// everything from the first date keyword onward is the due phrase,
		// minus any @labels / pN that appear within it
		var due []string
		for _, f := range fields[dueStart:] {
			low := strings.ToLower(f)
			if strings.HasPrefix(f, "@") || (len(low) == 2 && low[0] == 'p' && low[1] >= '1' && low[1] <= '4') {
				continue
			}
			due = append(due, f)
		}
		out.DueString = strings.Join(due, " ")
	}
	out.Content = strings.TrimSpace(strings.Join(contentWords, " "))
	if out.Content == "" {
		out.Content = strings.TrimSpace(text) // fallback: never create an empty task
	}
	return out
}

// startsDatePhrase reports whether the token at fields[i] begins a date phrase.
// Strong keywords and date-like literals qualify on their own; the ambiguous
// prepositions in duePrefixKeywords ("on", "in", "next", "every") qualify only
// when the following token is itself date-like — otherwise they're just English
// (e.g. "work on features", "interested in cooking").
func startsDatePhrase(fields []string, i int) bool {
	f := fields[i]
	low := strings.ToLower(f)
	if dueKeywords[low] || looksLikeDate(f) {
		return true
	}
	if duePrefixKeywords[low] {
		return i+1 < len(fields) && dateish(fields[i+1])
	}
	return false
}

// dateish reports whether a single token looks like part of a date: a strong
// date keyword, a date/time literal, a time-unit word, or a bare number or
// ordinal (3, 15, 3rd, 21st).
func dateish(s string) bool {
	low := strings.ToLower(s)
	if dueKeywords[low] || dueUnitWords[low] || looksLikeDate(s) {
		return true
	}
	num := low
	for _, suf := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(num, suf) {
			num = strings.TrimSuffix(num, suf)
			break
		}
	}
	if num == "" {
		return false
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func looksLikeDate(s string) bool {
	// YYYY-MM-DD, MM/DD, or a bare time like 9am / 9:30
	if len(s) >= 8 && s[4] == '-' {
		return true
	}
	if strings.Contains(s, "/") {
		return true
	}
	ls := strings.ToLower(s)
	return strings.HasSuffix(ls, "am") || strings.HasSuffix(ls, "pm") || strings.Contains(ls, ":")
}

// todoui talks to Todoist directly via the Sync API (no CLI). It keeps a local
// cache on disk so it works fully offline, and a queue of pending commands that
// are flushed to the server on sync.
const syncURL = "https://api.todoist.com/api/v1/sync"

// ---------- auth ----------

var cachedToken string

// tokenFromFile reads {"token":"…"} from a config.json path.
func tokenFromFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Token)
}

// Token reads the API token from, in order: $TODOIST_API_TOKEN,
// ~/.config/todoui/config.json, then ~/.config/todoist/config.json.
func Token() (string, error) {
	if cachedToken != "" {
		return cachedToken, nil
	}
	if t := strings.TrimSpace(os.Getenv("TODOIST_API_TOKEN")); t != "" {
		cachedToken = t
		return t, nil
	}
	if p := todouiConfigPath(); p != "" {
		if t := tokenFromFile(p); t != "" {
			cachedToken = t
			return t, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if t := tokenFromFile(filepath.Join(home, ".config", "todoist", "config.json")); t != "" {
			cachedToken = t
			return t, nil
		}
	}
	return "", fmt.Errorf("no token configured")
}

// todouiConfigPath is todoui's own config file (~/.config/todoui/config.json).
func todouiConfigPath() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "config.json")
}

// todoistConfigPath is where the token is stored (shared with the sachaos CLI).
func todoistConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "todoist")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SaveToken writes the token to todoui's config (~/.config/todoui/config.json)
// and, for CLI compatibility, also to ~/.config/todoist/config.json.
func SaveToken(t string) error {
	t = strings.TrimSpace(t)
	if t == "" {
		return fmt.Errorf("empty token")
	}
	b, _ := json.Marshal(map[string]string{"token": t})
	wrote := false
	if p := todouiConfigPath(); p != "" {
		if os.WriteFile(p, b, 0o600) == nil {
			wrote = true
		}
	}
	if p, err := todoistConfigPath(); err == nil {
		_ = os.WriteFile(p, b, 0o600) // best-effort CLI compatibility copy
		wrote = true
	}
	if !wrote {
		return fmt.Errorf("could not write token file")
	}
	cachedToken = t
	return nil
}

// HasToken reports whether a token is configured (env or file).
func HasToken() bool {
	_, err := Token()
	return err == nil
}

// ClearToken removes the saved token files and clears the cached token.
// (It cannot unset $TODOIST_API_TOKEN — that takes precedence if present.)
func ClearToken() {
	cachedToken = ""
	if p := todouiConfigPath(); p != "" {
		_ = os.Remove(p)
	}
	if p, err := todoistConfigPath(); err == nil {
		_ = os.Remove(p)
	}
}

// TokenFromEnv reports whether the token comes from the environment (which
// ClearToken can't remove).
func TokenFromEnv() bool {
	return strings.TrimSpace(os.Getenv("TODOIST_API_TOKEN")) != ""
}

// ClearLocalData removes the cached snapshot and the pending-command queue.
func ClearLocalData() {
	_ = os.Remove(cachePath())
	_ = os.Remove(queuePath())
}

// ValidateToken makes a cheap authenticated call. Returns (valid, authErr):
// authErr=true means the token was rejected (401/403); a network error returns
// (false,false) so callers can stay offline rather than force re-onboarding.
func ValidateToken() (valid bool, authErr bool) {
	token, err := Token()
	if err != nil {
		return false, true
	}
	req, err := http.NewRequest("GET", "https://api.todoist.com/api/v1/projects?limit=1", nil)
	if err != nil {
		return false, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return false, false // network/offline
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 200:
		return true, false
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return false, true
	default:
		return false, false
	}
}

// ---------- raw API model ----------

type apiDue struct {
	Date        string `json:"date"`
	String      string `json:"string"`
	IsRecurring bool   `json:"is_recurring"`
}

type apiDeadline struct {
	Date string `json:"date"` // YYYY-MM-DD
	Lang string `json:"lang"`
}

type apiItem struct {
	ID         string       `json:"id"`
	Content    string       `json:"content"`
	ProjectID  string       `json:"project_id"`
	Priority   int          `json:"priority"` // 1..4, 4 = highest (UI p1)
	Labels     []string     `json:"labels"`
	Due        *apiDue      `json:"due"`
	Deadline   *apiDeadline `json:"deadline"`
	Checked    bool         `json:"checked"`
	IsDeleted  bool         `json:"is_deleted"`
	AddedAt    string       `json:"added_at"`
	ChildOrder int          `json:"child_order"`
}

type apiProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsDeleted  bool   `json:"is_deleted"`
	IsArchived bool   `json:"is_archived"`
	ChildOrder int    `json:"child_order"`
}

type apiLabel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDeleted bool   `json:"is_deleted"`
}

type apiNote struct {
	ID        string `json:"id"`
	ItemID    string `json:"item_id"`
	Content   string `json:"content"`
	PostedAt  string `json:"posted_at"`
	IsDeleted bool   `json:"is_deleted"`
}

// ---------- local cache ----------

// Cache is the persisted local snapshot, keyed by id for incremental merges.
type Cache struct {
	SyncToken string                `json:"sync_token"`
	Items     map[string]apiItem    `json:"items"`
	Projects  map[string]apiProject `json:"projects"`
	Labels    map[string]apiLabel   `json:"labels"`
	Notes     map[string]apiNote    `json:"notes"`
}

func newCache() *Cache {
	return &Cache{
		SyncToken: "*",
		Items:     map[string]apiItem{},
		Projects:  map[string]apiProject{},
		Labels:    map[string]apiLabel{},
		Notes:     map[string]apiNote{},
	}
}

func cachePath() string { return filepath.Join(stateDir(), "cache.json") }
func queuePath() string { return filepath.Join(stateDir(), "queue.json") }

// LoadCache reads the on-disk cache, or returns an empty one.
func LoadCache() *Cache {
	b, err := os.ReadFile(cachePath())
	if err != nil {
		return newCache()
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return newCache()
	}
	if c.Items == nil {
		c.Items = map[string]apiItem{}
	}
	if c.Projects == nil {
		c.Projects = map[string]apiProject{}
	}
	if c.Labels == nil {
		c.Labels = map[string]apiLabel{}
	}
	if c.Notes == nil {
		c.Notes = map[string]apiNote{}
	}
	if c.SyncToken == "" {
		c.SyncToken = "*"
	}
	return &c
}

// Save writes the cache to disk.
func (c *Cache) Save() {
	if b, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(cachePath(), b, 0o600)
	}
}

// ---------- pending command queue ----------

// Command is one queued Sync API command.
type Command struct {
	Type   string         `json:"type"`
	UUID   string         `json:"uuid"`
	TempID string         `json:"temp_id,omitempty"`
	Args   map[string]any `json:"args"`
}

// LoadQueue reads the persisted pending commands.
func LoadQueue() []Command {
	b, err := os.ReadFile(queuePath())
	if err != nil {
		return nil
	}
	var q []Command
	if err := json.Unmarshal(b, &q); err != nil {
		return nil
	}
	return q
}

// SaveQueue persists the pending commands.
func SaveQueue(q []Command) {
	if b, err := json.Marshal(q); err == nil {
		_ = os.WriteFile(queuePath(), b, 0o600)
	}
}

// ---------- sync ----------

type syncResponse struct {
	SyncToken     string            `json:"sync_token"`
	FullSync      bool              `json:"full_sync"`
	Items         []apiItem         `json:"items"`
	Projects      []apiProject      `json:"projects"`
	Labels        []apiLabel        `json:"labels"`
	Notes         []apiNote         `json:"notes"`
	TempIDMapping map[string]string `json:"temp_id_mapping"`
}

// DoSync posts the queued commands and the sync token, returning the response.
func DoSync(syncToken string, commands []Command) (*syncResponse, error) {
	token, err := Token()
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("sync_token", syncToken)
	form.Set("resource_types", `["items","projects","labels","notes"]`)
	if len(commands) > 0 {
		cb, _ := json.Marshal(commands)
		form.Set("commands", string(cb))
	}
	req, err := http.NewRequest("POST", syncURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return nil, fmt.Errorf("sync %d: %s", resp.StatusCode, msg)
	}
	var sr syncResponse
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

// FilterTasks runs a Todoist filter query server-side (full filter grammar) and
// returns the matching items. Requires a network connection.
func FilterTasks(query string) ([]apiItem, error) {
	token, err := Token()
	if err != nil {
		return nil, err
	}
	var all []apiItem
	cursor := ""
	for {
		u := "https://api.todoist.com/api/v1/tasks/filter?limit=100&query=" + url.QueryEscape(query)
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			msg := strings.TrimSpace(string(data))
			if len(msg) > 160 {
				msg = msg[:160]
			}
			return nil, fmt.Errorf("filter %d: %s", resp.StatusCode, msg)
		}
		var out struct {
			Results    []apiItem `json:"results"`
			NextCursor *string   `json:"next_cursor"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Results...)
		if out.NextCursor == nil || *out.NextCursor == "" || len(all) >= 300 {
			break
		}
		cursor = *out.NextCursor
	}
	return all, nil
}

// FetchCompletedTasks pulls completed tasks from Todoist (last ~3 months),
// optionally scoped to one project. Completed records are mapped to apiItems
// flagged Checked so they slot straight into the cache / completed view.
func FetchCompletedTasks(projectID string) ([]apiItem, error) {
	token, err := Token()
	if err != nil {
		return nil, err
	}
	until := time.Now().UTC()
	since := until.AddDate(0, -3, 0) // endpoint allows at most a 3-month window
	var all []apiItem
	cursor := ""
	for {
		u := "https://api.todoist.com/api/v1/tasks/completed/by_completion_date" +
			"?since=" + url.QueryEscape(since.Format("2006-01-02T15:04:05")) +
			"&until=" + url.QueryEscape(until.Format("2006-01-02T15:04:05")) +
			"&limit=200"
		if projectID != "" {
			u += "&project_id=" + url.QueryEscape(projectID)
		}
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			msg := strings.TrimSpace(string(data))
			if len(msg) > 160 {
				msg = msg[:160]
			}
			return nil, fmt.Errorf("completed %d: %s", resp.StatusCode, msg)
		}
		var out struct {
			Items []struct {
				ID          string   `json:"id"`
				TaskID      string   `json:"task_id"`
				ProjectID   string   `json:"project_id"`
				Content     string   `json:"content"`
				CompletedAt string   `json:"completed_at"`
				Priority    int      `json:"priority"`
				Labels      []string `json:"labels"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		for _, it := range out.Items {
			id := it.TaskID
			if id == "" {
				id = it.ID
			}
			all = append(all, apiItem{
				ID:        id,
				Content:   it.Content,
				ProjectID: it.ProjectID,
				Priority:  it.Priority,
				Labels:    it.Labels,
				Checked:   true,
				AddedAt:   it.CompletedAt,
			})
		}
		if out.NextCursor == nil || *out.NextCursor == "" || len(all) >= 500 {
			break
		}
		cursor = *out.NextCursor
	}
	return all, nil
}

// Merge applies a sync response into the cache (full or incremental) and removes
// optimistic temp-id entries that now have real ids.
func (c *Cache) Merge(sr *syncResponse) {
	if sr.FullSync {
		c.Items = map[string]apiItem{}
		c.Projects = map[string]apiProject{}
		c.Labels = map[string]apiLabel{}
		c.Notes = map[string]apiNote{}
	}
	// Drop the optimistic temp-id placeholders now that the server has assigned
	// real ids. This must cover projects and labels too — otherwise a project
	// created via the mind map's T flow lingers under its tmp- id forever,
	// showing up as a ghost duplicate next to its real synced entry.
	for temp := range sr.TempIDMapping {
		delete(c.Items, temp)
		delete(c.Notes, temp)
		delete(c.Projects, temp)
		delete(c.Labels, temp)
	}
	for _, it := range sr.Items {
		if it.IsDeleted {
			delete(c.Items, it.ID)
		} else {
			c.Items[it.ID] = it
		}
	}
	for _, p := range sr.Projects {
		if p.IsDeleted {
			delete(c.Projects, p.ID)
		} else {
			c.Projects[p.ID] = p
		}
	}
	for _, l := range sr.Labels {
		if l.IsDeleted {
			delete(c.Labels, l.ID)
		} else {
			c.Labels[l.ID] = l
		}
	}
	for _, n := range sr.Notes {
		if n.IsDeleted {
			delete(c.Notes, n.ID)
		} else {
			c.Notes[n.ID] = n
		}
	}
	c.SyncToken = sr.SyncToken
}

// PruneOrphanTemps removes optimistic tmp- cache entries that are no longer
// backed by a pending command. An optimistic entry is only valid while the
// command that creates it sits in the queue; once that command has flushed and
// the real id has synced, any leftover tmp- entry is a ghost — e.g. a project
// created via the mind map's T flow before temp projects were cleaned on merge,
// which otherwise shows up as a duplicate next to its real synced entry.
// pending holds the TempIDs of commands still queued, which must be kept.
func (c *Cache) PruneOrphanTemps(pending map[string]bool) int {
	removed := 0
	orphan := func(id string) bool { return strings.HasPrefix(id, "tmp-") && !pending[id] }
	for id := range c.Projects {
		if orphan(id) {
			delete(c.Projects, id)
			removed++
		}
	}
	for id := range c.Items {
		if orphan(id) {
			delete(c.Items, id)
			removed++
		}
	}
	for id := range c.Notes {
		if orphan(id) {
			delete(c.Notes, id)
			removed++
		}
	}
	for id := range c.Labels {
		if orphan(id) {
			delete(c.Labels, id)
			removed++
		}
	}
	return removed
}

// pendingTempIDs collects the temp ids of commands still waiting in the queue,
// so PruneOrphanTemps keeps their optimistic cache entries.
func pendingTempIDs(queue []Command) map[string]bool {
	s := map[string]bool{}
	for _, cmd := range queue {
		if cmd.TempID != "" {
			s[cmd.TempID] = true
		}
	}
	return s
}

// ---------- translation to the display model ----------

func (c *Cache) projectName(id string) string {
	if p, ok := c.Projects[id]; ok {
		return "#" + p.Name
	}
	return ""
}

// toTask converts a cache item to the display Task.
func (c *Cache) toTask(it apiItem) Task {
	labels := ""
	if len(it.Labels) > 0 {
		labels = "@" + strings.Join(it.Labels, " @")
	}
	prio := it.Priority
	if prio < 1 || prio > 4 {
		prio = 1
	}
	deadline := ""
	if it.Deadline != nil {
		deadline = it.Deadline.Date
	}
	return Task{
		ID:        it.ID,
		Priority:  fmt.Sprintf("p%d", 5-prio), // API 4=highest → p1
		DueDate:   formatDue(it.Due),
		Deadline:  deadline,
		Project:   c.projectName(it.ProjectID),
		Labels:    labels,
		Content:   it.Content,
		Recurring: it.Due != nil && it.Due.IsRecurring,
	}
}

// formatDue renders the API due object into a sortable, readable string.
func formatDue(d *apiDue) string {
	if d == nil {
		return ""
	}
	s := d.Date // "2026-07-04" or "2026-07-04T09:00:00"
	if s == "" {
		return d.String
	}
	out := strings.Replace(s, "T", " ", 1)
	if len(out) >= 16 {
		out = out[:16] // trim seconds
	}
	if d.IsRecurring {
		out += " ↻"
	}
	return out
}

// AllTasks returns active (not checked/deleted) tasks from the cache.
func (c *Cache) AllTasks() []Task {
	items := make([]apiItem, 0, len(c.Items))
	for _, it := range c.Items {
		if it.Checked || it.IsDeleted {
			continue
		}
		items = append(items, it)
	}
	// stable order: by child_order then id, so the list doesn't jump around
	sort.Slice(items, func(i, j int) bool {
		if items[i].ChildOrder != items[j].ChildOrder {
			return items[i].ChildOrder < items[j].ChildOrder
		}
		return items[i].ID < items[j].ID
	})
	out := make([]Task, len(items))
	for i, it := range items {
		out[i] = c.toTask(it)
	}
	return out
}

// CompletedTasks returns completed (checked, not deleted) tasks known to the
// local cache, newest-added first. Each is flagged Done for read-only display.
func (c *Cache) CompletedTasks() []Task {
	items := make([]apiItem, 0)
	for _, it := range c.Items {
		if it.Checked && !it.IsDeleted {
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AddedAt > items[j].AddedAt })
	out := make([]Task, len(items))
	for i, it := range items {
		t := c.toTask(it)
		t.Done = true
		out[i] = t
	}
	return out
}

// RecentTaskIDs returns up to limit active task IDs, most recently added first.
func (c *Cache) RecentTaskIDs(limit int) []string {
	items := make([]apiItem, 0, len(c.Items))
	for _, it := range c.Items {
		if it.Checked || it.IsDeleted {
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AddedAt > items[j].AddedAt })
	ids := make([]string, 0, limit)
	for _, it := range items {
		ids = append(ids, it.ID)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

// CommentsFor returns the cached comments for a task, oldest first.
func (c *Cache) CommentsFor(taskID string) []Comment {
	var out []Comment
	for _, n := range c.Notes {
		if n.ItemID == taskID && !n.IsDeleted {
			out = append(out, Comment{ID: n.ID, Content: n.Content, PostedAt: n.PostedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PostedAt < out[j].PostedAt })
	return out
}

// Projects returns the projects for the picker, in Todoist order.
func (c *Cache) ProjectList() []Project {
	ps := make([]apiProject, 0, len(c.Projects))
	for _, p := range c.Projects {
		if !p.IsDeleted && !p.IsArchived {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].ChildOrder != ps[j].ChildOrder {
			return ps[i].ChildOrder < ps[j].ChildOrder
		}
		return ps[i].Name < ps[j].Name
	})
	out := make([]Project, len(ps))
	for i, p := range ps {
		out[i] = Project{ID: p.ID, Name: "#" + p.Name}
	}
	return out
}

// Comment is one task comment (shared with the UI).
type Comment struct {
	ID       string
	Content  string
	PostedAt string
}

// shortTime turns "2026-06-15T08:21:16.7Z" into "2026-06-15 08:21".
func shortTime(s string) string {
	if len(s) >= 16 {
		return strings.Replace(s[:16], "T", " ", 1)
	}
	return s
}
