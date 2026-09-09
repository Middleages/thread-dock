package workrun

import "testing"

func TestForegroundIdentifiersAreStableAndBounded(t *testing.T) {
	firstInvocation, firstLogical := foregroundIDs("work-1", "request-1")
	secondInvocation, secondLogical := foregroundIDs("work-1", "request-1")
	if firstInvocation != secondInvocation || firstLogical != secondLogical {
		t.Fatalf("foreground IDs are not deterministic: %q/%q vs %q/%q", firstInvocation, firstLogical, secondInvocation, secondLogical)
	}
	if len(firstInvocation) != 32 || len(firstLogical) != 32 || !isLowerHex(string(firstInvocation)) || !isLowerHex(string(firstLogical)) {
		t.Fatalf("foreground IDs are not bounded lowercase hex: %q/%q", firstInvocation, firstLogical)
	}
	if got := foregroundWorktreeID("work/with/secrets", "task/with/secrets"); len(got) != 32 || !isLowerHex(got) {
		t.Fatalf("worktree ID=%q", got)
	}
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
