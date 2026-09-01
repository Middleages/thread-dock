// Package opencodeagent validates the names of OpenCode Agents selected by
// ThreadDock role routing.
package opencodeagent

// ValidName reports whether value is a safe OpenCode Agent name. Names are
// limited to ASCII letters, digits, periods, underscores, and hyphens, and
// may be at most 64 bytes long.
func ValidName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}
