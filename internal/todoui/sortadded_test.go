package todoui

import "testing"

func TestSortByDateAdded(t *testing.T) {
	m := newTestModel()
	m.cache = newCache()
	m.cache.Items["a"] = apiItem{ID: "a", AddedAt: "2026-01-01T00:00:00Z"} // oldest
	m.cache.Items["b"] = apiItem{ID: "b", AddedAt: "2026-06-01T00:00:00Z"} // newest
	m.cache.Items["c"] = apiItem{ID: "c", AddedAt: "2026-03-01T00:00:00Z"} // middle

	ts := []Task{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.sortMode = sortAdded

	// Descending (↓): newest first — this is the default the app opens in.
	m.sortDesc = true
	m.sortTasks(ts)
	if got := []string{ts[0].ID, ts[1].ID, ts[2].ID}; got[0] != "b" || got[1] != "c" || got[2] != "a" {
		t.Fatalf("newest-first order wrong: %v", got)
	}

	// Ascending (↑): oldest first.
	m.sortDesc = false
	m.sortTasks(ts)
	if got := []string{ts[0].ID, ts[1].ID, ts[2].ID}; got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Fatalf("oldest-first order wrong: %v", got)
	}

	if sortAdded.label() != "date added" {
		t.Fatalf("label = %q", sortAdded.label())
	}
}

// TestDefaultSortIsNewestFirst locks in that a freshly built model opens sorted
// by created date descending (newest task on top).
func TestDefaultSortIsNewestFirst(t *testing.T) {
	m := initialModel()
	if m.sortMode != sortAdded {
		t.Fatalf("default sortMode = %v, want sortAdded", m.sortMode)
	}
	if !m.sortDesc {
		t.Fatalf("default sortDesc = false, want true (descending / newest first)")
	}
}
