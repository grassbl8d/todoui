package main

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
	Content    string
	Labels     []string
	Priority   int    // API priority 1..4 (4 = highest); 1 if unset
	DueString  string // natural-language due, parsed server-side on sync
}

var dueKeywords = map[string]bool{
	"today": true, "tomorrow": true, "tom": true, "tonight": true, "tonite": true,
	"yesterday": true, "next": true, "every": true, "in": true, "on": true,
	"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true,
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
	"friday": true, "saturday": true, "sunday": true,
	"jan": true, "feb": true, "mar": true, "apr": true, "may": true, "jun": true,
	"jul": true, "aug": true, "sep": true, "oct": true, "nov": true, "dec": true,
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
		case dueStart < 0 && (dueKeywords[low] || looksLikeDate(f)):
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

// Merge applies a sync response into the cache (full or incremental) and removes
// optimistic temp-id entries that now have real ids.
func (c *Cache) Merge(sr *syncResponse) {
	if sr.FullSync {
		c.Items = map[string]apiItem{}
		c.Projects = map[string]apiProject{}
		c.Labels = map[string]apiLabel{}
		c.Notes = map[string]apiNote{}
	}
	for _, real := range sr.TempIDMapping {
		_ = real
	}
	for temp := range sr.TempIDMapping {
		delete(c.Items, temp)
		delete(c.Notes, temp)
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
		if !p.IsDeleted {
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
