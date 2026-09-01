package opencodeagent

import (
	"strings"
	"testing"
)

func TestValidNameAcceptsOpenCodeRoleNames(t *testing.T) {
	for _, value := range []string{"build", "threaddock-builder", "review.v2", "review_agent"} {
		if !ValidName(value) {
			t.Fatalf("ValidName(%q)=false", value)
		}
	}
}

func TestValidNameRejectsUnsafeOrNonCanonicalValues(t *testing.T) {
	for _, value := range []string{"", " reviewer", "reviewer ", "review/agent", "리뷰어", "reviewer;touch", strings.Repeat("a", 65)} {
		if ValidName(value) {
			t.Fatalf("ValidName(%q)=true", value)
		}
	}
}
