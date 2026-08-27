package version

import "testing"

func TestDefaults(t *testing.T) {
	if Build != "dev" {
		t.Fatalf("Build = %q", Build)
	}
	if Contract != 1 {
		t.Fatalf("Contract = %d", Contract)
	}
}
