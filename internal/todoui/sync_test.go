package todoui

import "testing"

// A project created optimistically under a tmp- id must be removed once the
// real id arrives via temp_id_mapping — otherwise it lingers as a ghost
// duplicate next to its real synced entry (the bind/T-flow bug).
func TestMergeDropsTempProjectOnIDMapping(t *testing.T) {
	c := newCache()
	c.Projects["tmp-abc"] = apiProject{ID: "tmp-abc", Name: "Docker Compose Generator"}

	c.Merge(&syncResponse{
		TempIDMapping: map[string]string{"tmp-abc": "REAL123"},
		Projects:      []apiProject{{ID: "REAL123", Name: "Docker Compose Generator"}},
	})

	if _, ok := c.Projects["tmp-abc"]; ok {
		t.Fatal("the tmp- project should be removed after its id maps to a real one")
	}
	if _, ok := c.Projects["REAL123"]; !ok {
		t.Fatal("the real project should be present after merge")
	}
	n := 0
	for _, p := range c.Projects {
		if p.Name == "Docker Compose Generator" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one project by that name, got %d", n)
	}
}

// PruneOrphanTemps clears tmp- entries with no backing queued command, but keeps
// ones whose creating command is still pending (e.g. offline).
func TestPruneOrphanTemps(t *testing.T) {
	c := newCache()
	c.Projects["tmp-orphan"] = apiProject{ID: "tmp-orphan", Name: "Ghost"}
	c.Projects["tmp-pending"] = apiProject{ID: "tmp-pending", Name: "Still queued"}
	c.Projects["REAL"] = apiProject{ID: "REAL", Name: "Real"}

	queue := []Command{{Type: "project_add", TempID: "tmp-pending"}}
	removed := c.PruneOrphanTemps(pendingTempIDs(queue))

	if removed != 1 {
		t.Fatalf("expected to remove 1 orphan, removed %d", removed)
	}
	if _, ok := c.Projects["tmp-orphan"]; ok {
		t.Fatal("orphaned tmp- project should be pruned")
	}
	if _, ok := c.Projects["tmp-pending"]; !ok {
		t.Fatal("a tmp- project still backed by a queued command must be kept")
	}
	if _, ok := c.Projects["REAL"]; !ok {
		t.Fatal("real projects must never be pruned")
	}
}
