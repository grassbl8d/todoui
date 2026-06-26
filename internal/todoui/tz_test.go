package todoui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDefaultsTimezoneAndSync(t *testing.T) {
	d := defaultSettings()
	if d.Timezone != "Asia/Manila" {
		t.Fatalf("default timezone should be Asia/Manila, got %q", d.Timezone)
	}
	if d.SyncSeconds != 30 {
		t.Fatalf("default sync should be 30s, got %d", d.SyncSeconds)
	}
}

func TestTzOffsetLabelManila(t *testing.T) {
	// Manila is a fixed UTC+8 with no DST.
	if got := tzOffsetLabel("Asia/Manila"); got != "UTC+8" {
		t.Fatalf("Asia/Manila should be UTC+8, got %q", got)
	}
	if got := tzOffsetLabel("UTC"); got != "UTC+0" {
		t.Fatalf("UTC should be UTC+0, got %q", got)
	}
	if got := tzOffsetLabel("Not/AZone"); got != "" {
		t.Fatalf("invalid zone should yield empty label, got %q", got)
	}
}

func TestApplyTimezoneDrivesToday(t *testing.T) {
	orig := tz
	defer func() { tz = orig }()

	applyTimezone("Asia/Manila")
	loc, _ := time.LoadLocation("Asia/Manila")
	if got, want := todayStr(), time.Now().In(loc).Format("2006-01-02"); got != want {
		t.Fatalf("todayStr should follow the active tz: got %q want %q", got, want)
	}

	// An unknown zone must fall back, never panic or blank out tz.
	applyTimezone("Totally/Bogus")
	if tz == nil {
		t.Fatal("applyTimezone left tz nil for a bad zone")
	}
}

func TestAvailableTimezonesHasStaples(t *testing.T) {
	zones := availableTimezones()
	if len(zones) < 50 {
		t.Fatalf("expected a substantial zone list, got %d", len(zones))
	}
	want := []string{"Asia/Manila", "UTC", "Europe/London", "America/New_York"}
	have := map[string]bool{}
	for _, z := range zones {
		have[z] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Fatalf("zone list missing %q", w)
		}
	}
}

func TestTzFiltered(t *testing.T) {
	m := model{tzAll: []string{"Asia/Manila", "Asia/Tokyo", "Europe/London"}}
	m.tzQuery = "mani"
	got := m.tzFiltered()
	if len(got) != 1 || got[0] != "Asia/Manila" {
		t.Fatalf("filter 'mani' should yield only Asia/Manila, got %v", got)
	}
	m.tzQuery = "asia"
	if len(m.tzFiltered()) != 2 {
		t.Fatalf("filter 'asia' should match 2 zones, got %v", m.tzFiltered())
	}
	m.tzQuery = ""
	if len(m.tzFiltered()) != 3 {
		t.Fatal("empty query should return all zones")
	}
}

// TestTimezonePickerFlow drives the real key path: open Menu (,), move to the
// Timezone row, open the picker, type-to-filter, and select.
func TestTimezonePickerFlow(t *testing.T) {
	orig := tz
	defer func() { tz = orig }()
	// Isolate file IO (settings.Save) to a throwaway HOME so the real
	// ~/.config/todo-ui/settings.json is never touched by this test.
	t.Setenv("HOME", t.TempDir())

	m := newTestModel()
	m.width, m.height = 100, 40

	// , opens the Menu.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	m = nm.(model)
	if m.mode != modeOptions {
		t.Fatal(", should open the Menu")
	}
	// Timezone is the last row (index 5): move down to it.
	for i := 0; i < 5; i++ {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = nm.(model)
	}
	if m.optCursor != 5 {
		t.Fatalf("expected cursor on Timezone row (5), got %d", m.optCursor)
	}
	// Enter opens the searchable picker.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeTimezone {
		t.Fatal("enter on Timezone row should open the picker")
	}
	if len(m.tzAll) == 0 {
		t.Fatal("picker should have loaded the zone list")
	}
	if v := m.View(); !strings.Contains(v, "Timezone") || !strings.Contains(v, "type to filter") {
		t.Fatalf("picker view should render its header and footer, got:\n%s", v)
	}
	// Type "tokyo" to filter, then select.
	for _, r := range "tokyo" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	if got := m.tzFiltered(); len(got) == 0 || got[m.tzCursor] != "Asia/Tokyo" {
		t.Fatalf("filtering 'tokyo' should surface Asia/Tokyo, got %v", got)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeOptions {
		t.Fatal("enter should select and return to the Menu")
	}
	if m.settings.Timezone != "Asia/Tokyo" {
		t.Fatalf("timezone should be Asia/Tokyo, got %q", m.settings.Timezone)
	}
	if _, off := time.Now().In(tz).Zone(); off != 9*3600 {
		t.Fatalf("active tz should be UTC+9 after selecting Tokyo, got offset %d", off)
	}
}
