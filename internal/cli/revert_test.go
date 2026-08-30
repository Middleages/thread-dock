package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
)

type fakeReverter struct {
	calls  int
	id     contract.RunID
	reason string
	pr     github.PullRequest
	err    error
}

func (f *fakeReverter) CreateRevert(_ context.Context, id contract.RunID, reason string) (github.PullRequest, error) {
	f.calls++
	f.id, f.reason = id, reason
	return f.pr, f.err
}

func TestCreateRevertRoutesToInjectedService(t *testing.T) {
	fake := &fakeReverter{pr: github.PullRequest{Number: 201}}
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"create-revert", "run-184", "--reason", "pilot regression"}, &out, &errOut, Dependencies{Reverter: fake})
	if code != 0 || out.String() != "201\n" || fake.calls != 1 || fake.id != "run-184" || fake.reason != "pilot regression" || errOut.Len() != 0 {
		t.Fatalf("code=%d out=%q stderr=%q fake=%+v", code, out.String(), errOut.String(), fake)
	}
}

func TestCreateRevertRejectsMalformedInvocation(t *testing.T) {
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"create-revert", "run-184", "--reason"}, &out, &errOut, Dependencies{})
	if code != 2 || !strings.Contains(errOut.String(), "create-revert RUN --reason TEXT") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestCreateRevertReportsInjectedFailure(t *testing.T) {
	fake := &fakeReverter{err: errors.New("merge state missing")}
	var out, errOut strings.Builder
	code := RunWithDependencies(context.Background(), []string{"create-revert", "run-184", "--reason", "pilot regression"}, &out, &errOut, Dependencies{Reverter: fake})
	if code != 1 || !strings.Contains(errOut.String(), "merge state missing") {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}
