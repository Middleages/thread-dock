package pathscope

import "testing"

func TestNormalizeCleansSeparatorsAndDotSegments(t *testing.T) {
	got, err := Normalize(`.\src/./payments\api/**`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "src/payments/api/**" {
		t.Fatalf("normalized path = %q", got)
	}
}

func TestNormalizeRejectsParentTraversal(t *testing.T) {
	if _, err := Normalize(`src/../authentication/**`); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestNormalizeRejectsEmptySegmentsAndAbsolutePaths(t *testing.T) {
	for _, pattern := range []string{"", "./", "src//file.txt", "/src/file.txt", `C:\src\file.txt`, "src/"} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := Normalize(pattern); err == nil {
				t.Fatalf("expected invalid path %q", pattern)
			}
		})
	}
}

func TestOverlapsOnlyScopesThatCanOwnTheSamePath(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{left: "src/payments/**", right: "src/payments/api/**", want: true},
		{left: "src/payments/**", right: `src\payments\api.go`, want: true},
		{left: "pilot-result.txt", right: "pilot-result.txt", want: true},
		{left: "pilot-result.txt", right: "pilot-result.txt.bak", want: false},
		{left: "src/payments/**", right: "src/payments-api/**", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.left+"|"+tt.right, func(t *testing.T) {
			got, err := Overlaps(tt.left, tt.right)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("overlap = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsMatchesExactAndRecursiveScopes(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{pattern: `src\payments\api.go`, path: "src/payments/api.go", want: true},
		{pattern: "src/payments/**", path: "src/payments/api.go", want: true},
		{pattern: "src/payments/**", path: "src/payments-api/api.go", want: false},
		{pattern: "pilot-result.txt", path: "pilot-result.txt.bak", want: false},
		{pattern: "src/payments/**", path: "src/payments", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.path, func(t *testing.T) {
			if got := Contains(tt.pattern, tt.path); got != tt.want {
				t.Fatalf("contains = %v, want %v", got, tt.want)
			}
		})
	}
}
