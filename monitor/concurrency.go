package main

// concurrentRunCapable is an optional process capability. Keeping it outside
// CommandRunner lets deterministic test doubles remain serial unless they
// explicitly opt in.
type concurrentRunCapable interface {
	SupportsConcurrentRuns() bool
}

func supportsConcurrentRuns(process CommandRunner) bool {
	capable, ok := process.(concurrentRunCapable)
	return ok && capable.SupportsConcurrentRuns()
}
