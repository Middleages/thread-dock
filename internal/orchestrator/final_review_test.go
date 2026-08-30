package orchestrator

import (
	"context"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/state"
)

func TestTerminalFallbackRequiresExactPrefix(t *testing.T) {
	for _, tc := range []struct {
		identity string
		want     bool
	}{
		{"herdr-terminal:abc", true},
		{"provider-terminal-session", false},
		{"xherdr-terminal:abc", false},
		{"Herdr-terminal:abc", false},
	} {
		if got := isTerminalIdentity(tc.identity); got != tc.want {
			t.Fatalf("identity=%q got=%v want=%v", tc.identity, got, tc.want)
		}
	}
}

func TestCIRepairAttributionRequiresUniqueRequiredCommand(t *testing.T) {
	c := contract.TaskContract{Tasks: []contract.Task{
		{ID: "api", Verification: []string{"go test ./internal/payments"}},
		{ID: "tests", Verification: []string{"go test ./tests/payments"}},
	}}
	if id, ok := uniqueCIRepairTask(c, github.CheckState{Name: "go test ./internal/payments", State: "failure"}); !ok || id != "api" {
		t.Fatalf("unique attribution=%q,%v", id, ok)
	}
	if id, ok := uniqueCIRepairTask(c, github.CheckState{Name: "ci", State: "failure"}); ok || id != "" {
		t.Fatalf("ambiguous attribution=%q,%v", id, ok)
	}
}

type recordingFingerprintReader struct {
	*fakeGit
	path string
}

func (r *recordingFingerprintReader) Fingerprint(_ context.Context, path string) (string, error) {
	r.path = path
	return "managed", nil
}

func TestRecoveryFingerprintReadsBuilderWorktree(t *testing.T) {
	reader := &recordingFingerprintReader{fakeGit: &fakeGit{}}
	o := &Orchestrator{deps: Dependencies{Worktree: reader}}
	_, err := o.builderManagedFingerprint(context.Background(), state.TaskRunState{Worktree: state.WorktreeState{Path: "/builder/worktree"}})
	if err != nil || reader.path != "/builder/worktree" {
		t.Fatalf("path=%q err=%v", reader.path, err)
	}
}

func TestProtectedConfirmationInvalidatesEvidenceBeforeResume(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseNeedsOperator
	snapshot.ProtectedReasons = []string{"authentication"}
	snapshot.FinalSHA = validSHA
	snapshot.FinalChecksSHA = validSHA
	snapshot.FinalChecks = []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}}
	snapshot.MainSHA = validSHA
	snapshot.MergeabilityKnown, snapshot.Mergeable = true, true
	snapshot.ActionCursor = 9
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseCI || got.FinalSHA != "" || got.FinalChecksSHA != "" || got.MainSHA != "" || got.PullRequestHeadSHA != "" || got.MergeabilityKnown || got.MergePreflightReady || got.CIState != "" {
		t.Fatalf("confirmation did not invalidate evidence: %+v", got)
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got2 := h.mustLoad(id); got2.UpdatedAt != got.UpdatedAt {
		t.Fatalf("repeated confirmation changed snapshot: before=%+v after=%+v", got, got2)
	}
}

func TestConflictAbortIsAttemptedBeforeBlocking(t *testing.T) {
	h := newParallelHarness(t)
	h.mergeConflict = true
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if h.mustLoad(id).Phase == contract.PhaseBlocked {
			break
		}
	}
	if h.parallelGit.abortMerges == 0 {
		t.Fatal("confirmed conflict was blocked without merge abort")
	}
}
