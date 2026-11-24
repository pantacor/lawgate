//go:build !lawgate_bite

package bite

// Off is disabled in ordinary builds.
func Off(string) bool { return false }
