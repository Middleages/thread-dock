// Package pathscope defines the repository-relative path scopes used by task
// ownership and protected-path checks.
package pathscope

import (
	"fmt"
	"strings"
)

// Normalize validates and canonicalizes a repository-relative path scope.
// Exact paths identify one path; a scope ending in /** identifies that
// directory and the paths below it.
func Normalize(pattern string) (string, error) {
	scope, err := parse(pattern)
	if err != nil {
		return "", err
	}
	if scope.recursive {
		return scope.base + "/**", nil
	}
	return scope.base, nil
}

// Overlaps reports whether two scopes could own the same repository path.
func Overlaps(left, right string) (bool, error) {
	leftScope, err := parse(left)
	if err != nil {
		return false, err
	}
	rightScope, err := parse(right)
	if err != nil {
		return false, err
	}

	switch {
	case !leftScope.recursive && !rightScope.recursive:
		return leftScope.base == rightScope.base, nil
	case leftScope.recursive && rightScope.recursive:
		return isDirectoryPrefix(leftScope.base, rightScope.base) || isDirectoryPrefix(rightScope.base, leftScope.base), nil
	case leftScope.recursive:
		return containsPath(leftScope.base, rightScope.base), nil
	default:
		return containsPath(rightScope.base, leftScope.base), nil
	}
}

// Contains reports whether path is inside pattern. Invalid patterns or paths
// return false because this predicate cannot return an error.
func Contains(pattern, path string) bool {
	patternScope, err := parse(pattern)
	if err != nil {
		return false
	}
	pathScope, err := parse(path)
	if err != nil || pathScope.recursive {
		return false
	}
	if !patternScope.recursive {
		return patternScope.base == pathScope.base
	}
	return containsPath(patternScope.base, pathScope.base)
}

type scope struct {
	base      string
	recursive bool
}

func parse(pattern string) (scope, error) {
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	if pattern == "" || strings.HasPrefix(pattern, "/") || isWindowsAbsolute(pattern) {
		return scope{}, fmt.Errorf("path scope must be repository-relative")
	}

	recursive := strings.HasSuffix(pattern, "/**")
	base := pattern
	if recursive {
		base = strings.TrimSuffix(base, "/**")
	}
	if base == "" {
		return scope{}, fmt.Errorf("path scope must name a path")
	}

	parts := strings.Split(base, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "":
			return scope{}, fmt.Errorf("path scope contains an empty segment")
		case ".":
			continue
		case "..":
			return scope{}, fmt.Errorf("path scope contains parent traversal")
		}
		if strings.Contains(part, "*") {
			return scope{}, fmt.Errorf("path scope contains unsupported wildcard")
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return scope{}, fmt.Errorf("path scope must name a path")
	}
	return scope{base: strings.Join(cleaned, "/"), recursive: recursive}, nil
}

func isWindowsAbsolute(pattern string) bool {
	return len(pattern) >= 2 && pattern[1] == ':'
}

func isDirectoryPrefix(parent, child string) bool {
	return parent == child || strings.HasPrefix(child, parent+"/")
}

func containsPath(directory, path string) bool {
	return isDirectoryPrefix(directory, path)
}
