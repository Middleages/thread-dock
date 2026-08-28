package orchestrator

import (
	"strings"
	"testing"

	"thread-dock/internal/herdr"
	"thread-dock/internal/testfixture"
)

func TestBuilderPacketRequiresCommittedTaskBranchSHA(t *testing.T) {
	c := testfixture.ValidContract()
	task := c.Tasks[0]
	requestID := "run-37:builder-prompt"

	packet := builderPacket(c, task, requestID)
	lowerPacket := strings.ToLower(packet)

	for _, want := range []string{
		"before submitting final evidence",
		"stage and commit all allowed changes on the task branch",
		"after committing",
		"git rev-parse HEAD",
		"40-character lowercase SHA",
		"Do not submit the base commit",
		"Run each exact required verification command against the committed result before final evidence",
	} {
		if !strings.Contains(lowerPacket, strings.ToLower(want)) {
			t.Errorf("Builder packet missing %q: %q", want, packet)
		}
	}

	if strings.Contains(packet, "git commit -m") {
		t.Error("Builder packet hardcodes a commit message")
	}
	if strings.Contains(packet, "git add ") {
		t.Error("Builder packet hardcodes a staging command")
	}
	if !strings.Contains(packet, herdr.THREADDOCK_EVIDENCE_BEGIN) || !strings.Contains(packet, herdr.EvidenceSchemaExample) || !strings.Contains(packet, herdr.THREADDOCK_EVIDENCE_END) {
		t.Fatalf("Builder packet changed evidence envelope: %q", packet)
	}
	if !strings.Contains(packet, "Use requestId="+requestID) {
		t.Fatalf("Builder packet missing request ID: %q", packet)
	}
	for _, criterion := range append(append([]string{}, c.Parent.AcceptanceCriteria...), task.AcceptanceCriteria...) {
		if !strings.Contains(packet, criterion) {
			t.Fatalf("Builder packet missing acceptance criterion %q: %q", criterion, packet)
		}
	}
}
