package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"thread-dock/internal/contract"
)

type fakeConfirmer struct {
	calls int
	id    contract.RunID
	err   error
}

func (f *fakeConfirmer) ConfirmProtectedChange(_ context.Context, id contract.RunID) error {
	f.calls++
	f.id = id
	return f.err
}

func TestConfirmProtectedChangeRoutesToInjectedService(t *testing.T) {
	fake := &fakeConfirmer{}
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"confirm", "run-184", "protected-change"}, &out, &errOut, Dependencies{Confirmer: fake})
	if code != 0 || fake.calls != 1 || fake.id != "run-184" || errOut.Len() != 0 {
		t.Fatalf("code=%d calls=%d id=%q stderr=%q", code, fake.calls, fake.id, errOut.String())
	}
}

func TestConfirmProtectedChangeRejectsMalformedInvocation(t *testing.T) {
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"confirm", "run-184", "wrong"}, &out, &errOut, Dependencies{})
	if code != 2 || !strings.Contains(errOut.String(), "confirm RUN protected-change") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestConfirmProtectedChangeReportsInjectedFailure(t *testing.T) {
	fake := &fakeConfirmer{err: errors.New("not operator-ready")}
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"confirm", "run-184", "protected-change"}, &out, &errOut, Dependencies{Confirmer: fake})
	if code != 1 || !strings.Contains(errOut.String(), "not operator-ready") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestNeedsProductionDependenciesForConfirmButNotRemovedRevert(t *testing.T) {
	if !NeedsProductionDependencies([]string{"confirm", "run-184", "protected-change"}) {
		t.Fatal("confirm should require production dependencies")
	}
	if NeedsProductionDependencies([]string{"create-revert", "run-184", "--reason", "pilot regression"}) {
		t.Fatal("removed create-revert must not require production dependencies")
	}
}
