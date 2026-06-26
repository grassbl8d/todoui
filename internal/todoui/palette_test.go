package todoui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPaletteOpensAndRunsAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate settings.Save() triggered by the action

	m := newTestModel()
	m.width, m.height = 100, 40
	m.list.SetSize(100, 36)
	wasLight := m.settings.Light

	// ` opens the quick-action palette.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("`")})
	m = nm.(model)
	if m.mode != modePalette {
		t.Fatal("` should open the quick-action palette")
	}

	// Type to filter down to the theme toggle.
	for _, r := range "theme" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	got := m.palFiltered()
	if len(got) != 1 || got[0].key != "+" {
		t.Fatalf("filtering 'theme' should surface the + action, got %v", got)
	}
	if v := m.View(); !strings.Contains(v, "Quick action") || !strings.Contains(v, "enter run") {
		t.Fatalf("palette view should render header + footer, got:\n%s", v)
	}

	// Enter runs it: closes the palette and toggles the theme.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.mode != modeList {
		t.Fatal("enter should close the palette back to the list")
	}
	if m.settings.Light == wasLight {
		t.Fatal("running the theme action should have toggled the theme")
	}
}

func TestPaletteEmptyQueryAndFilter(t *testing.T) {
	m := newTestModel()
	if len(m.palFiltered()) != len(paletteActions) {
		t.Fatal("empty query should return every action")
	}
	m.palQuery = "due"
	due := m.palFiltered()
	if len(due) < 3 {
		t.Fatalf("'due' should match several actions, got %d", len(due))
	}
	m.palQuery = "o" // exact key match
	if len(m.palFiltered()) == 0 {
		t.Fatal("a single-letter key query should still match its action")
	}
}

func TestAboutScreen(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 100, 40

	// ~ opens the about screen.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("~")})
	m = nm.(model)
	if m.mode != modeAbout {
		t.Fatal("~ should open the about screen")
	}
	v := m.View()
	if !strings.Contains(v, "Carlo C.") || !strings.Contains(v, "todo-ui "+version) {
		t.Fatalf("about view should show contributors and version, got:\n%s", v)
	}
	if !strings.Contains(v, "█") {
		t.Fatal("about view should render the block-letter logo")
	}

	// Any key closes it.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = nm.(model)
	if m.mode != modeList {
		t.Fatal("any key should close the about screen")
	}
}
