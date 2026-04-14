package store

import (
	"encoding/json"
	"testing"
)

// ─── Conflict Resolution Tests ────────────────────────────────────────────────

// TestNewerRemoteMutationAppliesWithoutConflict verifies that a remote mutation
// with a strictly higher revision applies cleanly and bumps the local revision.
func TestNewerRemoteMutationAppliesWithoutConflict(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed: create a local observation via sync pull with revision 1.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-1",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-1","session_id":"s1","type":"decision","title":"v1","content":"original","project":"engram","scope":"project","revision":1}`,
	}); err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	// Apply a remote mutation with revision 2 (newer) — should succeed.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-1",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-1","session_id":"s1","type":"decision","title":"v2","content":"updated","project":"engram","scope":"project","revision":2}`,
	}); err != nil {
		t.Fatalf("apply newer remote: %v", err)
	}

	obs, err := s.GetObservationBySyncID("obs-conflict-1")
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if obs.Title != "v2" {
		t.Fatalf("expected title 'v2' after newer remote apply, got %q", obs.Title)
	}
	if obs.Revision < 2 {
		t.Fatalf("expected revision >= 2 after newer remote apply, got %d", obs.Revision)
	}

	// No conflicts should have been created.
	conflicts, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts after newer remote apply, got %d", len(conflicts))
	}
}

// TestOlderRemoteMutationGeneratesConflict verifies that a remote mutation with
// a revision <= local revision creates a sync_conflict row and does NOT
// overwrite local data.
func TestOlderRemoteMutationGeneratesConflict(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed: create a local observation at revision 3.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-2",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-2","session_id":"s1","type":"decision","title":"local-v3","content":"local data v3","project":"engram","scope":"project","revision":3}`,
	}); err != nil {
		t.Fatalf("seed observation at rev 3: %v", err)
	}

	// Attempt to apply an older remote mutation at revision 2.
	// This should generate a conflict, NOT overwrite.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-2",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-2","session_id":"s1","type":"decision","title":"remote-v2","content":"remote stale data","project":"engram","scope":"project","revision":2}`,
	}); err != nil {
		t.Fatalf("apply older remote (should not error, should conflict): %v", err)
	}

	// Local data must be unchanged.
	obs, err := s.GetObservationBySyncID("obs-conflict-2")
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if obs.Title != "local-v3" {
		t.Fatalf("expected local title preserved after conflict, got %q", obs.Title)
	}
	if obs.Revision != 3 {
		t.Fatalf("expected local revision preserved (3), got %d", obs.Revision)
	}

	// A conflict should exist.
	conflicts, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.Entity != SyncEntityObservation {
		t.Fatalf("expected entity observation, got %q", c.Entity)
	}
	if c.EntityKey != "obs-conflict-2" {
		t.Fatalf("expected entity_key obs-conflict-2, got %q", c.EntityKey)
	}
	if c.LocalRevision != 3 {
		t.Fatalf("expected local_revision 3, got %d", c.LocalRevision)
	}
	if c.RemoteRevision != 2 {
		t.Fatalf("expected remote_revision 2, got %d", c.RemoteRevision)
	}
	if c.Project != "engram" {
		t.Fatalf("expected project engram, got %q", c.Project)
	}

	// Verify local_data and remote_data contain the expected payloads.
	var localPayload syncObservationPayload
	if err := json.Unmarshal([]byte(c.LocalData), &localPayload); err != nil {
		t.Fatalf("unmarshal local data: %v", err)
	}
	if localPayload.Title != "local-v3" {
		t.Fatalf("expected local_data title 'local-v3', got %q", localPayload.Title)
	}

	var remotePayload syncObservationPayload
	if err := json.Unmarshal([]byte(c.RemoteData), &remotePayload); err != nil {
		t.Fatalf("unmarshal remote data: %v", err)
	}
	if remotePayload.Title != "remote-v2" {
		t.Fatalf("expected remote_data title 'remote-v2', got %q", remotePayload.Title)
	}
}

// TestSameRevisionGeneratesConflict verifies that a remote mutation with the
// SAME revision as local (equal, not just less) also generates a conflict.
func TestSameRevisionGeneratesConflict(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed at revision 2.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-same",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-same","session_id":"s1","type":"decision","title":"local","content":"local","project":"engram","scope":"project","revision":2}`,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Apply remote also at revision 2.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-conflict-same",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-conflict-same","session_id":"s1","type":"decision","title":"remote","content":"remote","project":"engram","scope":"project","revision":2}`,
	}); err != nil {
		t.Fatalf("apply same revision: %v", err)
	}

	conflicts, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict for equal revision, got %d", len(conflicts))
	}
}

// TestLegacyPayloadWithoutRevisionBypassesConflictCheck verifies backward
// compatibility: payloads with revision=0 (legacy) always apply without
// conflict detection.
func TestLegacyPayloadWithoutRevisionBypassesConflictCheck(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed at revision 5.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-legacy",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-legacy","session_id":"s1","type":"decision","title":"local","content":"local","project":"engram","scope":"project","revision":5}`,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Apply legacy payload WITHOUT revision field → should apply (backward compat).
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-legacy",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-legacy","session_id":"s1","type":"decision","title":"legacy-update","content":"from old client","project":"engram","scope":"project"}`,
	}); err != nil {
		t.Fatalf("apply legacy payload: %v", err)
	}

	obs, err := s.GetObservationBySyncID("obs-legacy")
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if obs.Title != "legacy-update" {
		t.Fatalf("expected legacy payload to apply, got title %q", obs.Title)
	}

	// No conflicts from legacy payloads.
	conflicts, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts from legacy payload, got %d", len(conflicts))
	}
}

// TestListSyncConflictsFiltersByProject verifies that ListSyncConflicts can
// filter by project and only returns unresolved conflicts.
func TestListSyncConflictsFiltersByProject(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Create observations in two projects.
	for i, p := range []struct {
		syncID, project string
	}{
		{"obs-alpha", "alpha"},
		{"obs-beta", "beta"},
	} {
		if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
			Seq:       int64(i + 1),
			TargetKey: DefaultSyncTargetKey,
			Entity:    SyncEntityObservation,
			EntityKey: p.syncID,
			Op:        SyncOpUpsert,
			Payload:   `{"sync_id":"` + p.syncID + `","session_id":"s1","type":"decision","title":"v1","content":"data","project":"` + p.project + `","scope":"project","revision":3}`,
		}); err != nil {
			t.Fatalf("seed %s: %v", p.syncID, err)
		}
	}

	// Generate conflicts for both.
	for _, p := range []struct {
		syncID string
		seq    int64
	}{
		{"obs-alpha", 3},
		{"obs-beta", 4},
	} {
		if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
			Seq:       p.seq,
			TargetKey: DefaultSyncTargetKey,
			Entity:    SyncEntityObservation,
			EntityKey: p.syncID,
			Op:        SyncOpUpsert,
			Payload:   `{"sync_id":"` + p.syncID + `","session_id":"s1","type":"decision","title":"stale","content":"stale","project":"` + (map[string]string{"obs-alpha": "alpha", "obs-beta": "beta"})[p.syncID] + `","scope":"project","revision":1}`,
		}); err != nil {
			t.Fatalf("conflict %s: %v", p.syncID, err)
		}
	}

	// List all conflicts.
	all, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total conflicts, got %d", len(all))
	}

	// Filter by project alpha.
	alpha, err := s.ListSyncConflicts("alpha")
	if err != nil {
		t.Fatalf("list alpha: %v", err)
	}
	if len(alpha) != 1 {
		t.Fatalf("expected 1 alpha conflict, got %d", len(alpha))
	}
	if alpha[0].Project != "alpha" {
		t.Fatalf("expected alpha project, got %q", alpha[0].Project)
	}

	// Filter by project beta.
	beta, err := s.ListSyncConflicts("beta")
	if err != nil {
		t.Fatalf("list beta: %v", err)
	}
	if len(beta) != 1 {
		t.Fatalf("expected 1 beta conflict, got %d", len(beta))
	}
	if beta[0].Project != "beta" {
		t.Fatalf("expected beta project, got %q", beta[0].Project)
	}

	// Filter by nonexistent project.
	none, err := s.ListSyncConflicts("nonexistent")
	if err != nil {
		t.Fatalf("list nonexistent: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 conflicts for nonexistent project, got %d", len(none))
	}
}

// TestResolveConflictLocalWins verifies that resolving with "local" strategy
// keeps local data unchanged and marks the conflict as resolved.
func TestResolveConflictLocalWins(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed at revision 3.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-local-wins",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-local-wins","session_id":"s1","type":"decision","title":"local-title","content":"local content","project":"engram","scope":"project","revision":3}`,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Generate conflict with older remote.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-local-wins",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-local-wins","session_id":"s1","type":"decision","title":"remote-title","content":"remote content","project":"engram","scope":"project","revision":1}`,
	}); err != nil {
		t.Fatalf("generate conflict: %v", err)
	}

	conflicts, _ := s.ListSyncConflicts("")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict before resolve, got %d", len(conflicts))
	}
	conflictID := conflicts[0].ID

	// Resolve with local_wins.
	if err := s.ResolveConflict(conflictID, ConflictResolutionLocal); err != nil {
		t.Fatalf("resolve local: %v", err)
	}

	// Local data unchanged.
	obs, err := s.GetObservationBySyncID("obs-local-wins")
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if obs.Title != "local-title" {
		t.Fatalf("expected local title preserved after local_wins, got %q", obs.Title)
	}
	if obs.Content != "local content" {
		t.Fatalf("expected local content preserved after local_wins, got %q", obs.Content)
	}

	// Conflict is resolved — no longer in pending list.
	pending, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list pending after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending conflicts after resolve, got %d", len(pending))
	}

	// Cannot resolve the same conflict again.
	if err := s.ResolveConflict(conflictID, ConflictResolutionLocal); err == nil {
		t.Fatalf("expected error when resolving already-resolved conflict")
	}
}

// TestResolveConflictRemoteWins verifies that resolving with "remote" strategy
// applies remote data to the observation and marks the conflict as resolved.
func TestResolveConflictRemoteWins(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed at revision 3.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-remote-wins",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-remote-wins","session_id":"s1","type":"decision","title":"local-title","content":"local content","project":"engram","scope":"project","revision":3}`,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Generate conflict with older remote.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       2,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-remote-wins",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-remote-wins","session_id":"s1","type":"decision","title":"remote-title","content":"remote content","project":"engram","scope":"project","revision":1}`,
	}); err != nil {
		t.Fatalf("generate conflict: %v", err)
	}

	conflicts, _ := s.ListSyncConflicts("")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict before resolve, got %d", len(conflicts))
	}
	conflictID := conflicts[0].ID

	// Resolve with remote_wins.
	if err := s.ResolveConflict(conflictID, ConflictResolutionRemote); err != nil {
		t.Fatalf("resolve remote: %v", err)
	}

	// Remote data applied.
	obs, err := s.GetObservationBySyncID("obs-remote-wins")
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if obs.Title != "remote-title" {
		t.Fatalf("expected remote title after remote_wins, got %q", obs.Title)
	}
	if obs.Content != "remote content" {
		t.Fatalf("expected remote content after remote_wins, got %q", obs.Content)
	}
	// Revision should be bumped past both local and remote.
	if obs.Revision < 4 {
		t.Fatalf("expected revision >= 4 after remote_wins resolve, got %d", obs.Revision)
	}

	// Conflict is resolved.
	pending, err := s.ListSyncConflicts("")
	if err != nil {
		t.Fatalf("list pending after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending conflicts after resolve, got %d", len(pending))
	}
}

// TestResolveConflictRejectsInvalidStrategy verifies that invalid resolution
// strategies are rejected.
func TestResolveConflictRejectsInvalidStrategy(t *testing.T) {
	s := newTestStore(t)

	if err := s.ResolveConflict(1, "invalid_strategy"); err == nil {
		t.Fatalf("expected error for invalid resolution strategy")
	}
}

// TestResolveConflictNotFound verifies that resolving a non-existent conflict
// returns an error.
func TestResolveConflictNotFound(t *testing.T) {
	s := newTestStore(t)

	if err := s.ResolveConflict(99999, ConflictResolutionLocal); err == nil {
		t.Fatalf("expected error for non-existent conflict")
	}
}

// TestConflictDetectionForNewInsert verifies that inserting a brand-new
// observation via sync pull sets the revision correctly.
func TestConflictDetectionForNewInsert(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Insert a new observation via sync pull with revision 5.
	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-new-rev",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-new-rev","session_id":"s1","type":"decision","title":"new","content":"new observation","project":"engram","scope":"project","revision":5}`,
	}); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	obs, err := s.GetObservationBySyncID("obs-new-rev")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if obs.Revision != 5 {
		t.Fatalf("expected revision 5 on new insert, got %d", obs.Revision)
	}
}

// TestNewInsertWithZeroRevisionDefaultsToOne verifies that inserting a new
// observation without revision defaults to revision 1.
func TestNewInsertWithZeroRevisionDefaultsToOne(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateSession("s1", "engram", "/tmp/engram"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := s.ApplyPulledMutation(DefaultSyncTargetKey, SyncMutation{
		Seq:       1,
		TargetKey: DefaultSyncTargetKey,
		Entity:    SyncEntityObservation,
		EntityKey: "obs-zero-rev",
		Op:        SyncOpUpsert,
		Payload:   `{"sync_id":"obs-zero-rev","session_id":"s1","type":"decision","title":"zero","content":"zero rev","project":"engram","scope":"project"}`,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	obs, err := s.GetObservationBySyncID("obs-zero-rev")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if obs.Revision != 1 {
		t.Fatalf("expected revision 1 for new insert without revision, got %d", obs.Revision)
	}
}
