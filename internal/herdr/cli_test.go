package herdr

import (
	"context"
	"embed"
	"errors"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/runner"
)

// testFS contains captured Herdr v0.8.2 wire responses.
//
//go:embed testdata/v0.8.2/*.txt
var testFS embed.FS

type recordingRunner struct {
	responses map[string]string
	calls     [][]string
	cwds      []string
	fail      *runnerFailure
}

type orderedResponse struct {
	call   []string
	stdout string
}

type orderedRunner struct {
	responses []orderedResponse
	calls     [][]string
	cwds      []string
}

func (r *orderedRunner) Run(_ context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	call := append([]string{executable}, args...)
	r.calls = append(r.calls, call)
	r.cwds = append(r.cwds, cwd)
	if len(r.responses) == 0 {
		return runner.Result{ExitCode: 1}, &testError{"unexpected command after fixture responses"}
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	if !reflect.DeepEqual(call, response.call) {
		return runner.Result{ExitCode: 1}, &testError{"unexpected command order"}
	}
	return runner.Result{Stdout: response.stdout, ExitCode: 0}, nil
}

func (r *recordingRunner) Run(_ context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	call := append([]string{executable}, args...)
	r.calls = append(r.calls, call)
	r.cwds = append(r.cwds, cwd)
	if r.fail != nil {
		return runner.Result{Stderr: r.fail.stderr, ExitCode: r.fail.exitCode}, &testError{r.fail.err}
	}
	key := strings.Join(call, "\x00")
	response, ok := r.responses[key]
	if !ok {
		return runner.Result{ExitCode: 1}, &testError{"unexpected command: " + strings.Join(call, " ")}
	}
	return runner.Result{Stdout: response, ExitCode: 0}, nil
}

type runnerFailure struct {
	stderr   string
	err      string
	exitCode int
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

type stdoutErrorRunner struct{}

func (stdoutErrorRunner) Run(context.Context, string, string, ...string) (runner.Result, error) {
	return runner.Result{Stdout: `{"error":{"code":"agent_not_found","message":"not found"}}`, ExitCode: 1}, &testError{"command failed"}
}

func TestCreateWorktreeReturnsActualIDsAndUsesExplicitArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00agent/184-integration\x00--base\x00main\x00--label\x00issue-184-integration\x00--no-focus": readFixture(t, "testdata/v0.8.2/worktree-create.txt"),
		"herdr\x00pane\x00list\x00--workspace\x00workspace-redacted": readFixture(t, "testdata/v0.8.2/pane-list.txt"),
	})
	cli := NewCLI(r, "herdr")

	got, err := cli.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Cwd: "/repo", Branch: "agent/184-integration", Base: "main", Label: "issue-184-integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != "workspace-redacted" || got.PaneID != "pane-redacted" {
		t.Fatalf("result=%#v", got)
	}
	if got.Path != "/redacted/worktree" {
		t.Fatalf("path=%q", got.Path)
	}
	want := [][]string{
		{"herdr", "worktree", "create", "--cwd", "/repo", "--branch", "agent/184-integration", "--base", "main", "--label", "issue-184-integration", "--no-focus"},
		{"herdr", "pane", "list", "--workspace", "workspace-redacted"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
}

func TestAgentLifecycleCommandsUseStructuredArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00start\x00builder_api\x00--kind\x00opencode\x00--pane\x00pane-redacted":                           `{}`,
		"herdr\x00agent\x00prompt\x00builder_api\x00packet with spaces\n$(not-a-command)\x00--wait\x00--timeout\x003600000": `{}`,
		"herdr\x00agent\x00get\x00builder_api":                                                    readFixture(t, "testdata/v0.8.2/agent-get.txt"),
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": "recent output",
	})
	cli := NewCLI(r, "herdr")

	if err := cli.StartAgent(context.Background(), StartAgentRequest{Name: "builder_api", PaneID: "pane-redacted"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.Prompt(context.Background(), "builder_api", "packet with spaces\n$(not-a-command)"); err != nil {
		t.Fatal(err)
	}
	state, err := cli.Get(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if state != AgentStateWorking {
		t.Fatalf("state=%q", state)
	}
	recent, err := cli.ReadRecent(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if recent != "recent output" {
		t.Fatalf("recent=%q", recent)
	}
}

func TestRunnerFailureDoesNotExposeSecrets(t *testing.T) {
	packetSecret := "packet-secret-184"
	r := &recordingRunner{fail: &runnerFailure{
		stderr: "stderr-secret-184", err: "runner-secret-184", exitCode: 17,
	}}
	err := NewCLI(r, "herdr").Prompt(context.Background(), "builder_api", packetSecret)
	if err == nil {
		t.Fatal("expected prompt error")
	}
	message := err.Error()
	for _, secret := range []string{packetSecret, "stderr-secret-184", "runner-secret-184", "builder_api"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error contains secret %q: %q", secret, message)
		}
	}
	if message != "herdr agent prompt failed (exit code 17)" {
		t.Fatalf("error=%q", message)
	}
}

func TestReadRecentReturnsRawStdout(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": readFixture(t, "testdata/v0.8.2/recent-output.txt"),
	})
	got, err := NewCLI(r, "herdr").ReadRecent(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if got != readFixture(t, "testdata/v0.8.2/recent-output.txt") {
		t.Fatalf("output=%q", got)
	}
}

func TestReadEvidenceAcceptsOnlyStructuredResultsWithDuration(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": EvidenceBeginMarker + "\n" + `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"2.3s"}]}` + "\n" + EvidenceEndMarker,
	})
	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil || got.CommitSHA == "" || got.Verification[0].Duration != "2.3s" {
		t.Fatalf("evidence=%#v err=%v", got, err)
	}
}

func TestReadReviewEvidenceAcceptsStrictBlockEnvelope(t *testing.T) {
	payload := `{"requestId":"review-1","decision":"block","blockingFindings":[{"id":"F-1","summary":"missing test","paths":["internal/api.go"]}],"riskCategories":["data"]}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00reviewer_api\x00--source\x00recent-unwrapped\x00--lines\x00120": THREADDOCK_REVIEW_BEGIN + "\n" + payload + "\n" + THREADDOCK_REVIEW_END,
	})
	got, err := NewCLI(r, "herdr").ReadReviewEvidence(context.Background(), "reviewer_api", "review-1")
	if err != nil || got.Decision != "block" || len(got.BlockingFindings) != 1 || got.BlockingFindings[0].Paths[0] != "internal/api.go" {
		t.Fatalf("evidence=%#v err=%v", got, err)
	}
}

func TestReadReviewEvidenceAcceptsStrictAcceptEnvelope(t *testing.T) {
	payload := `{"requestId":"review-accept","decision":"accept","blockingFindings":[],"riskCategories":["public_contract"]}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00reviewer_api\x00--source\x00recent-unwrapped\x00--lines\x00120": THREADDOCK_REVIEW_BEGIN + "\n" + payload + "\n" + THREADDOCK_REVIEW_END,
	})
	got, err := NewCLI(r, "herdr").ReadReviewEvidence(context.Background(), "reviewer_api", "review-accept")
	if err != nil || got.Decision != "accept" || len(got.BlockingFindings) != 0 || len(got.RiskCategories) != 1 {
		t.Fatalf("evidence=%#v err=%v", got, err)
	}
}

func TestReadReviewEvidenceRejectsAcceptWithFindingsAndStaleRequest(t *testing.T) {
	for name, payload := range map[string]string{
		"accept findings": `{"requestId":"review-1","decision":"accept","blockingFindings":[{"id":"F-1","summary":"x","paths":["src/api.go"]}],"riskCategories":[]}`,
		"stale request":   `{"requestId":"review-old","decision":"accept","blockingFindings":[]}`,
		"raw only":        "review accepted",
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00reviewer_api\x00--source\x00recent-unwrapped\x00--lines\x00120": THREADDOCK_REVIEW_BEGIN + "\n" + payload + "\n" + THREADDOCK_REVIEW_END,
			})
			if _, err := NewCLI(r, "herdr").ReadReviewEvidence(context.Background(), "reviewer_api", "review-1"); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestReadReviewEvidenceRejectsEveryStrictnessGuard(t *testing.T) {
	validFinding := `{"id":"F-1","summary":"missing test","paths":["internal/api.go"]}`
	validAccept := `{"requestId":"review-1","decision":"accept","blockingFindings":[],"riskCategories":[]}`
	cases := map[string]string{
		"unknown field":     `{"requestId":"review-1","decision":"accept","blockingFindings":[],"riskCategories":[],"extra":true}`,
		"trailing data":     `{"requestId":"review-1","decision":"accept","blockingFindings":[],"riskCategories":[]} trailing`,
		"block no findings": `{"requestId":"review-1","decision":"block","blockingFindings":[],"riskCategories":[]}`,
		"duplicate IDs":     `{"requestId":"review-1","decision":"block","blockingFindings":[` + validFinding + `,` + validFinding + `],"riskCategories":[]}`,
		"unknown risk":      `{"requestId":"review-1","decision":"accept","blockingFindings":[],"riskCategories":["unknown"]}`,
		"duplicate risk":    `{"requestId":"review-1","decision":"accept","blockingFindings":[],"riskCategories":["data","data"]}`,
		"noncanonical path": `{"requestId":"review-1","decision":"block","blockingFindings":[{"id":"F-1","summary":"x","paths":["./internal/api.go"]}],"riskCategories":[]}`,
		"credential":        `{"requestId":"review-1","decision":"block","blockingFindings":[{"id":"F-1","summary":"token: ghp_supersecret","paths":["internal/api.go"]}],"riskCategories":[]}`,
		"NUL":               "{\"requestId\":\"review-1\",\"decision\":\"accept\",\"blockingFindings\":[],\"riskCategories\":[]}\x00",
		"oversized":         `{"requestId":"review-1","decision":"block","blockingFindings":[{"id":"F-1","summary":"` + strings.Repeat("x", MaxEvidencePayloadBytes) + `","paths":["internal/api.go"]}],"riskCategories":[]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00reviewer_api\x00--source\x00recent-unwrapped\x00--lines\x00120": THREADDOCK_REVIEW_BEGIN + "\n" + payload + "\n" + THREADDOCK_REVIEW_END,
			})
			if _, err := NewCLI(r, "herdr").ReadReviewEvidence(context.Background(), "reviewer_api", "review-1"); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	for name, output := range map[string]string{
		"missing envelope": validAccept,
		"unmatched end":    THREADDOCK_REVIEW_END,
		"incomplete begin": THREADDOCK_REVIEW_BEGIN + "\n" + validAccept,
		"nested begin":     THREADDOCK_REVIEW_BEGIN + "\n" + THREADDOCK_REVIEW_BEGIN + "\n" + validAccept + "\n" + THREADDOCK_REVIEW_END,
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00reviewer_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadReviewEvidence(context.Background(), "reviewer_api", "review-1"); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestReadEvidenceAcceptsLiveSingletonVerificationObject(t *testing.T) {
	const payload = `{"requestId":"run-26:builder-prompt","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":{"command":"test -f pilot-result.txt","outcome":"passed","duration":"1ms"}}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": EvidenceBeginMarker + "\n" + payload + "\n" + EvidenceEndMarker,
	})

	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got.Verification) != 1 {
		t.Fatalf("verification=%#v, want one check", got.Verification)
	}
	check := got.Verification[0]
	if check.Command != "test -f pilot-result.txt" || check.Outcome != "passed" || check.Duration != "1ms" {
		t.Fatalf("check=%#v, want live pilot check", check)
	}
}

func TestReadEvidenceExtractsIndentedActualEnvelopeFromOpenCodeColumns(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": readFixture(t, "testdata/v0.8.2/recent-output-opencode-columns.txt"),
	})

	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.RequestID != "run-28:builder-prompt" || got.CommitSHA != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("evidence=%#v, want actual response envelope", got)
	}
	if len(got.Verification) != 1 || got.Verification[0].Command != "test -f pilot-result.txt" {
		t.Fatalf("verification=%#v, want singleton actual check", got.Verification)
	}
}

func TestReadEvidenceExtractsLastDynamicSuffixEnvelope(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": readFixture(t, "testdata/v0.8.2/recent-output-opencode-dynamic-suffix.txt"),
	})

	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.RequestID != "run-32:latest" || got.CommitSHA != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("evidence=%#v, want latest actual envelope", got)
	}
	if len(got.Verification) != 1 || got.Verification[0].Command != "test -f pilot-result.txt" {
		t.Fatalf("verification=%#v, want latest singleton check", got.Verification)
	}
}

func TestReadEvidenceRejectsDynamicMarkerSuffixWithoutExactSidebarBoundary(t *testing.T) {
	valid := `{"requestId":"run-32:negative","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	const boundary = 40
	markerLine := func(marker, suffix string, offset int) string {
		left := "    " + marker
		target := boundary + offset
		return left + strings.Repeat(" ", target-len(left)) + suffix
	}
	cases := map[string]string{
		"no boundary":       "    " + EvidenceBeginMarker + "\n    " + valid + "\n" + markerLine(EvidenceEndMarker, "6% used", 0) + "\n",
		"misaligned suffix": strings.Repeat(" ", boundary) + "Context\n" + "    " + EvidenceBeginMarker + "\n    " + valid + "\n" + markerLine(EvidenceEndMarker, "6% used", 1) + "\n",
		"prompt echo":       strings.Repeat(" ", boundary) + "Context\n  ┃  " + EvidenceBeginMarker + "\n  ┃  " + valid + "\n  ┃  " + markerLine(EvidenceEndMarker, "6% used", 0) + "\n",
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted dynamic marker suffix without an exact actual-pane boundary")
			}
		})
	}
}

func TestReadEvidenceDoesNotRetroactivelyUseSidebarBoundary(t *testing.T) {
	valid := `{"requestId":"run-32:retroactive","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	const boundary = 40
	markerLine := func(marker, suffix string, target int) string {
		left := "    " + marker
		return left + strings.Repeat(" ", target-len(left)) + suffix
	}
	cases := map[string]string{
		"sidebar inside envelope": markerLine(EvidenceBeginMarker, "dynamic begin", boundary) + "\n" +
			"    " + valid + "\n" + strings.Repeat(" ", boundary) + "Context\n" +
			"    " + EvidenceEndMarker + "\n",
		"sidebar after envelope": markerLine(EvidenceBeginMarker, "dynamic begin", boundary) + "\n" +
			"    " + valid + "\n    " + EvidenceEndMarker + "\n" +
			strings.Repeat(" ", boundary) + "Context\n",
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted dynamic marker using a retroactively inferred sidebar boundary")
			}
		})
	}
}

func TestReadEvidenceAllowsOnlyProspectiveSidebarBoundaryForLaterEnvelope(t *testing.T) {
	valid := `{"requestId":"run-32:prospective","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	const boundary = 40
	markerLine := func(marker, suffix string) string {
		left := "    " + marker
		return left + strings.Repeat(" ", boundary-len(left)) + suffix
	}
	output := "    " + EvidenceBeginMarker + "\n    " + valid + "\n    " + EvidenceEndMarker + "\n" +
		strings.Repeat(" ", boundary) + "Context\n" +
		markerLine(EvidenceBeginMarker, "dynamic begin") + "\n    " + valid + "\n" +
		markerLine(EvidenceEndMarker, "dynamic end") + "\n"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil || got.RequestID != "run-32:prospective" {
		t.Fatalf("evidence=%#v err=%v, want later envelope after boundary", got, err)
	}
}

func TestReadEvidenceRejectsPromptEchoOnlyEnvelope(t *testing.T) {
	output := "  ┃  " + EvidenceBeginMarker + "\n" +
		"  ┃  {\"requestId\":\"prompt-only\",\"commitSha\":\"0123456789abcdef0123456789abcdef01234567\",\"verification\":[]}" + "\n" +
		"  ┃  " + EvidenceEndMarker + "\n"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
		t.Fatal("accepted prompt echo as actual evidence")
	}
}

func TestReadEvidenceRejectsLeftPaneTrailingDataAfterObject(t *testing.T) {
	valid := `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	output := "    " + EvidenceBeginMarker + "\n    " + valid + "\n    {}\n    " + EvidenceEndMarker + "\n"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
		t.Fatal("accepted left-pane trailing JSON")
	}
}

func TestReadEvidenceRejectsLeftPaneTrailingTextAfterObjectOnSameLine(t *testing.T) {
	valid := `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	output := "    " + EvidenceBeginMarker + "\n    " + valid + "        trailing left-pane text\n    " + EvidenceEndMarker + "\n"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
		t.Fatal("accepted left-pane trailing text separated by spaces")
	}
}

func TestReadEvidenceRejectsAlignedLeftPaneTrailingJSONDespiteAuxiliaryMarkers(t *testing.T) {
	valid := `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	output := "    " + EvidenceBeginMarker + "        Context\n" +
		"    " + valid + "        {}\n" +
		"    " + EvidenceEndMarker + "        Context\n"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
		t.Fatal("accepted aligned left-pane trailing JSON as auxiliary text")
	}
}

func TestReadEvidenceBalancesBracesInsideEscapedJSONStrings(t *testing.T) {
	payload := `{"requestId":"run-28:quoted","commitSha":"abcdef0123456789abcdef0123456789abcdef01","verification":[{"command":"printf \"{\\\"nested\\\":true}\"","outcome":"passed","duration":"1ms"}]}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": EvidenceBeginMarker + "\n" + payload + "\n" + EvidenceEndMarker,
	})
	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got.Verification[0].Command != `printf "{\"nested\":true}"` {
		t.Fatalf("command=%q, want escaped braces preserved", got.Verification[0].Command)
	}
}

func TestReadEvidenceRejectsNestedAndUnmatchedActualMarkers(t *testing.T) {
	valid := `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	for name, output := range map[string]string{
		"nested":          EvidenceBeginMarker + "\n" + EvidenceBeginMarker + "\n" + valid + "\n" + EvidenceEndMarker + "\n" + EvidenceEndMarker,
		"unmatched begin": EvidenceBeginMarker + "\n" + valid,
		"unmatched end":   EvidenceEndMarker + "\n" + valid,
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted malformed actual marker sequence")
			}
		})
	}
}

func TestReadEvidenceRejectsInvalidSingletonVerificationShapes(t *testing.T) {
	const prefix = `{"requestId":"run-26:builder-prompt","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":`
	for name, verification := range map[string]string{
		"null":                    `null`,
		"scalar":                  `"test -f pilot-result.txt"`,
		"empty array":             `[]`,
		"malformed object":        `{"command":`,
		"multi-command shorthand": `{"command":["test -f pilot-result.txt","go test ./..."],"outcome":"passed","duration":"1ms"}`,
		"unknown field":           `{"command":"test -f pilot-result.txt","outcome":"passed","duration":"1ms","extra":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload := prefix + verification + `}`
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": EvidenceBeginMarker + "\n" + payload + "\n" + EvidenceEndMarker,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted invalid singleton verification shape")
			}
		})
	}
}

func TestReadEvidenceExtractsLastCompleteEnvelopeFromUITranscript(t *testing.T) {
	output := "recent UI output\n" + EvidenceBeginMarker + "\n" +
		`{"requestId":"prompt-old","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./old","outcome":"passed","duration":"1s"}]}` +
		"\n" + EvidenceEndMarker + "\nmore UI\n" + EvidenceBeginMarker + "\n" +
		`{"requestId":"prompt-last","commitSha":"abcdef0123456789abcdef0123456789abcdef01","verification":[{"command":"go test ./...","outcome":"passed","duration":"2s"}]}` +
		"\n" + EvidenceEndMarker + "\ntrailing UI text"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil || got.RequestID != "prompt-last" || got.Verification[0].Command != "go test ./..." {
		t.Fatalf("evidence=%#v err=%v", got, err)
	}
}

func TestReadEvidenceAcceptsTheCapturedRecentOutputFixtureAroundEnvelope(t *testing.T) {
	output := readFixture(t, "testdata/v0.8.2/recent-output.txt") + "\n" + EvidenceBeginMarker + "\n" +
		`{"requestId":"prompt-fixture","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}` +
		"\n" + EvidenceEndMarker + "\n" + readFixture(t, "testdata/v0.8.2/recent-output.txt")
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
	})
	if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err != nil {
		t.Fatal(err)
	}
}

func TestReadEvidenceRejectsMalformedIncompleteOversizedAndSecretEnvelopes(t *testing.T) {
	valid := `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}`
	cases := map[string]string{
		"malformed":            EvidenceBeginMarker + "\n{" + "\n" + EvidenceEndMarker,
		"incomplete":           EvidenceBeginMarker + "\n" + valid + "\n" + EvidenceBeginMarker,
		"oversized":            EvidenceBeginMarker + "\n" + strings.Repeat("x", MaxEvidencePayloadBytes+1) + "\n" + EvidenceEndMarker,
		"secret":               EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", "THREADDOCK_GH_TOKEN=plain-internal-token", 1) + "\n" + EvidenceEndMarker,
		"quoted token":         EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", `{"token":"plain-internal-token"}`, 1) + "\n" + EvidenceEndMarker,
		"quoted password":      EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", `{"password":"hunter2"}`, 1) + "\n" + EvidenceEndMarker,
		"quoted authorization": EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", `{"authorization":"Bearer internal-token"}`, 1) + "\n" + EvidenceEndMarker,
		"quoted assignment":    EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", `'secret' = 'value'`, 1) + "\n" + EvidenceEndMarker,
		"quoted client secret": EvidenceBeginMarker + "\n" + strings.Replace(valid, "go test ./...", `{"clientSecret": "value"}`, 1) + "\n" + EvidenceEndMarker,
		"unknown":              EvidenceBeginMarker + "\n" + `{"requestId":"prompt-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}],"extra":"not allowed"}` + "\n" + EvidenceEndMarker,
		"trailing":             EvidenceBeginMarker + "\n" + valid + "\n{}\n" + EvidenceEndMarker,
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted invalid evidence envelope")
			}
		})
	}
}

func TestEvidenceCredentialPatternRecognizesQuotedAssignments(t *testing.T) {
	for _, value := range []string{
		`{"token":"plain-internal-token"}`,
		`{"password":"hunter2"}`,
		`{"authorization":"Bearer internal-token"}`,
		`'secret' = 'value'`,
		`"clientSecret": "value"`,
	} {
		if !evidenceCredentialPattern.MatchString(value) {
			t.Errorf("evidenceCredentialPattern.MatchString(%q)=false, want true", value)
		}
	}
}

func TestFindWorktreeReconcilesPathWorkspaceAndPane(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00list\x00--cwd\x00/repo":           `{"result":{"worktrees":[{"branch":"agent/run-integration","label":"run","open_workspace_id":"workspace-run","path":"/work/run"}]}}`,
		"herdr\x00pane\x00list\x00--workspace\x00workspace-run": `{"result":{"panes":[{"pane_id":"pane-run","workspace_id":"workspace-run"}]}}`,
	})
	got, found, err := NewCLI(r, "herdr").FindWorktree(context.Background(), "/repo", "/work/run", "run")
	if err != nil || !found || got.Path != "/work/run" || got.WorkspaceID != "workspace-run" || got.PaneID != "pane-run" {
		t.Fatalf("worktree=%#v found=%v err=%v", got, found, err)
	}
}

func TestFindWorktreePropagatesPaneLookupFailure(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00list\x00--cwd\x00/repo": `{"result":{"worktrees":[{"label":"run","open_workspace_id":"workspace-run","path":"/work/run"}]}}`,
	})
	_, _, err := NewCLI(r, "herdr").FindWorktree(context.Background(), "/repo", "/work/run", "run")
	if err == nil || !strings.Contains(err.Error(), "pane list") {
		t.Fatalf("err=%v", err)
	}
}

func TestFindWorktreeReportsClosedWorkspace(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00list\x00--cwd\x00/repo": `{"result":{"worktrees":[{"label":"run","path":"/work/run"}]}}`,
	})
	_, _, err := NewCLI(r, "herdr").FindWorktree(context.Background(), "/repo", "/work/run", "run")
	if !errors.Is(err, ErrClosedWorkspace) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetInfoClassifiesAgentNotFoundFromStructuredStdout(t *testing.T) {
	_, err := NewCLI(stdoutErrorRunner{}, "herdr").GetInfo(context.Background(), "threaddock-definitely-missing-agent")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetInfoParsesStateChangeSequence(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder_api": readFixture(t, "testdata/v0.8.2/agent-get.txt"),
	})
	got, err := NewCLI(r, "herdr").GetInfo(context.Background(), "builder_api")
	if err != nil || got.StateChangeSeq != 110 {
		t.Fatalf("info=%#v err=%v", got, err)
	}
}

func TestGetInfoUsesNamespacedTerminalIdentityWhenOpenCodeSessionIsAbsent(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder-opencode": readFixture(t, "testdata/v0.8.2/agent-get-opencode-terminal-only.txt"),
	})

	got, err := NewCLI(r, "herdr").GetInfo(context.Background(), "builder-opencode")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "herdr-terminal:term-opencode-builder" {
		t.Fatalf("session id=%q, want namespaced terminal identity", got.SessionID)
	}
}

func TestGetInfoPrefersProviderSessionIdentityOverTerminalIdentity(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder_api": readFixture(t, "testdata/v0.8.2/agent-get.txt"),
	})

	got, err := NewCLI(r, "herdr").GetInfo(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-redacted" {
		t.Fatalf("session id=%q, want provider session identity", got.SessionID)
	}
}

func TestGetInfoRejectsAgentWithoutProviderSessionOrTerminalIdentity(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder-opencode": readFixture(t, "testdata/v0.8.2/agent-get-opencode-no-identity.txt"),
	})

	_, err := NewCLI(r, "herdr").GetInfo(context.Background(), "builder-opencode")
	if err == nil || err.Error() != "herdr agent get failed (exit code 0)" {
		t.Fatalf("err=%v, want safe identity error", err)
	}
}

func TestReadPromptReceiptReturnsSequenceAndOnlyRequestObservation(t *testing.T) {
	requestID := "run-184:builder-prompt"
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder-184":                                                    readFixture(t, "testdata/v0.8.2/agent-get.txt"),
		"herdr\x00agent\x00read\x00builder-184\x00--source\x00recent-unwrapped\x00--lines\x00120": "agent output with " + requestID,
	})
	info, observed, err := NewCLI(r, "herdr").ReadPromptReceipt(context.Background(), "builder-184", requestID)
	if err != nil || info.StateChangeSeq != 110 || !observed {
		t.Fatalf("info=%#v observed=%v err=%v", info, observed, err)
	}
	if len(r.calls) != 2 || !reflect.DeepEqual(r.calls[0], []string{"herdr", "agent", "read", "builder-184", "--source", "recent-unwrapped", "--lines", "120"}) || !reflect.DeepEqual(r.calls[1], []string{"herdr", "agent", "get", "builder-184"}) {
		t.Fatalf("calls=%#v", r.calls)
	}
	if !reflect.DeepEqual(r.cwds, []string{"", ""}) {
		t.Fatalf("cwds=%#v", r.cwds)
	}
}

func TestReadPromptReceiptReadsRecentBeforeLatestSequence(t *testing.T) {
	requestID := "run-184:builder-prompt"
	r := &orderedRunner{responses: []orderedResponse{
		{call: []string{"herdr", "agent", "read", "builder-184", "--source", "recent-unwrapped", "--lines", "120"}, stdout: readFixture(t, "testdata/v0.8.2/prompt-reconcile-recent-missing.txt")},
		{call: []string{"herdr", "agent", "get", "builder-184"}, stdout: readFixture(t, "testdata/v0.8.2/agent-get-seq-43.txt")},
	}}
	info, observed, err := NewCLI(r, "herdr").ReadPromptReceipt(context.Background(), "builder-184", requestID)
	if err != nil || info.StateChangeSeq != 43 || observed {
		t.Fatalf("info=%#v observed=%v err=%v", info, observed, err)
	}
	if !reflect.DeepEqual(r.cwds, []string{"", ""}) {
		t.Fatalf("cwds=%#v", r.cwds)
	}
}

func TestReadEvidenceRejectsRawTranscriptAndMissingDuration(t *testing.T) {
	for name, output := range map[string]string{
		"raw":      "commit_sha: 0123456789abcdef0123456789abcdef01234567",
		"duration": `{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed"}]}`,
		"trailing": `{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}\nnot-json`,
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted untrusted evidence")
			}
		})
	}
}

func TestStartAgentAlwaysStartsOpenCode(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00start\x00builder_api\x00--kind\x00opencode\x00--pane\x00pane-redacted": `{}`,
	})
	if err := NewCLI(r, "herdr").StartAgent(context.Background(), StartAgentRequest{Name: "builder_api", PaneID: "pane-redacted"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateWorktreeRejectsMismatchedWorkspaceRelationships(t *testing.T) {
	create := `{"result":{"root_pane":{"pane_id":"pane-a","workspace_id":"workspace-b"},"workspace":{"workspace_id":"workspace-a"}}}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00branch\x00--base\x00main\x00--label\x00label\x00--no-focus": create,
	})
	_, err := NewCLI(r, "herdr").CreateWorktree(context.Background(), CreateWorktreeRequest{Cwd: "/repo", Branch: "branch", Base: "main", Label: "label"})
	if err == nil || err.Error() != "herdr worktree create failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateWorktreeRequiresPaneWorkspaceMatch(t *testing.T) {
	create := `{"result":{"root_pane":{"pane_id":"pane-a","workspace_id":"workspace-a"},"workspace":{"workspace_id":"workspace-a"}}}`
	panes := `{"result":{"panes":[{"pane_id":"pane-a","workspace_id":"workspace-b"}]}}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00branch\x00--base\x00main\x00--label\x00label\x00--no-focus": create,
		"herdr\x00pane\x00list\x00--workspace\x00workspace-a":                                                                     panes,
	})
	_, err := NewCLI(r, "herdr").CreateWorktree(context.Background(), CreateWorktreeRequest{Cwd: "/repo", Branch: "branch", Base: "main", Label: "label"})
	if err == nil || err.Error() != "herdr pane list failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func TestAgentStatesAreLifecycleOnly(t *testing.T) {
	for input, want := range map[string]AgentState{
		"working": AgentStateWorking, "blocked": AgentStateBlocked, "idle": AgentStateIdle,
		"done": AgentStateDone, "unknown": AgentStateUnknown, "other": AgentStateUnknown,
	} {
		if got := ParseAgentState(input); got != want {
			t.Errorf("ParseAgentState(%q)=%q want %q", input, got, want)
		}
	}
}

func TestMalformedHerdrOutputReturnsError(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder_api": `{"id":"cli:agent:get","result":{}}`,
	})
	_, err := NewCLI(r, "herdr").Get(context.Background(), "builder_api")
	if err == nil || err.Error() != "herdr agent get failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func fixtureRunner(t *testing.T, responses map[string]string) *recordingRunner {
	t.Helper()
	return &recordingRunner{responses: responses}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := testFS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
