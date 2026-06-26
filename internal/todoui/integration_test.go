//go:build integration

// Live Todoist API integration guard. These tests hit the real Todoist server,
// so they're behind the `integration` build tag and excluded from the normal
// `go test ./...` run. They verify the endpoints todo-ui depends on still
// respond and parse — the integration most likely to break.
//
// Run them with a valid token:
//
//	scripts/integration-test.sh
//	# or directly:
//	TODOIST_API_TOKEN=… go test -tags integration -run Integration -v .
//
// Read-only by default. Set TODOUI_INTEGRATION_WRITE=1 to also run the
// create→complete→reopen→delete round-trip (it creates a clearly-named task in
// your Inbox and deletes it again, but it does touch your live account).
package todoui

import (
	"os"
	"testing"
	"time"
)

// requireToken skips the test when no Todoist token is configured.
func requireToken(t *testing.T) {
	t.Helper()
	if !HasToken() {
		t.Skip("no Todoist token (set TODOIST_API_TOKEN) — skipping integration test")
	}
}

func TestIntegrationValidateToken(t *testing.T) {
	requireToken(t)
	valid, authErr := ValidateToken()
	if authErr {
		t.Fatal("token was rejected by Todoist (401/403) — is TODOIST_API_TOKEN valid?")
	}
	if !valid {
		t.Fatal("could not validate token (network error or unexpected status)")
	}
}

func TestIntegrationSyncReadsState(t *testing.T) {
	requireToken(t)
	resp, err := DoSync("*", nil) // full sync
	if err != nil {
		t.Fatalf("full sync failed: %v", err)
	}
	if !resp.FullSync {
		t.Fatal("expected a full-sync response (full_sync=true)")
	}
	if resp.SyncToken == "" {
		t.Fatal("sync response had no sync_token — response shape may have changed")
	}
	// A real account should have at least the Inbox project.
	if len(resp.Projects) == 0 {
		t.Fatal("sync returned no projects — resource shape may have changed")
	}
	t.Logf("sync ok: %d projects, %d items", len(resp.Projects), len(resp.Items))
}

func TestIntegrationFilterTasks(t *testing.T) {
	requireToken(t)
	// "today" is always a valid filter; it parsing without error is the guard.
	if _, err := FilterTasks("today"); err != nil {
		t.Fatalf("filter query failed: %v", err)
	}
}

func TestIntegrationFetchCompleted(t *testing.T) {
	requireToken(t)
	// The completed-tasks endpoint is new and the most fragile; a successful,
	// parseable response (even if empty) is the guard.
	items, err := FetchCompletedTasks("")
	if err != nil {
		t.Fatalf("fetching completed tasks failed: %v", err)
	}
	t.Logf("completed endpoint ok: %d task(s) in the last 3 months", len(items))
}

func TestIntegrationTaskRoundTrip(t *testing.T) {
	requireToken(t)
	if os.Getenv("TODOUI_INTEGRATION_WRITE") != "1" {
		t.Skip("set TODOUI_INTEGRATION_WRITE=1 to run the write round-trip (creates & deletes a task)")
	}

	content := "todo-ui integration check — safe to delete (" +
		time.Now().UTC().Format("2006-01-02 15:04:05") + ")"
	tempID := "tmp-" + genID()

	// Create.
	add := Command{Type: "item_add", UUID: genID(), TempID: tempID,
		Args: map[string]any{"content": content}}
	resp, err := DoSync("*", []Command{add})
	if err != nil {
		t.Fatalf("item_add sync failed: %v", err)
	}
	realID := resp.TempIDMapping[tempID]
	if realID == "" {
		t.Fatalf("no temp_id_mapping for the created task — got %+v", resp.TempIDMapping)
	}
	t.Logf("created task %s", realID)

	// Always try to delete it again, even if a step below fails.
	defer func() {
		del := Command{Type: "item_delete", UUID: genID(), Args: map[string]any{"id": realID}}
		if _, err := DoSync(resp.SyncToken, []Command{del}); err != nil {
			t.Errorf("cleanup: failed to delete test task %s: %v", realID, err)
		} else {
			t.Logf("cleaned up task %s", realID)
		}
	}()

	// Complete, then reopen — exercises the commands the mind-map / list use.
	complete := Command{Type: "item_complete", UUID: genID(), Args: map[string]any{"id": realID}}
	if _, err := DoSync(resp.SyncToken, []Command{complete}); err != nil {
		t.Fatalf("item_complete failed: %v", err)
	}
	reopen := Command{Type: "item_uncomplete", UUID: genID(), Args: map[string]any{"id": realID}}
	if _, err := DoSync(resp.SyncToken, []Command{reopen}); err != nil {
		t.Fatalf("item_uncomplete failed: %v", err)
	}
}
