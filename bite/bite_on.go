//go:build lawgate_bite

package bite

import "os"

// Off reports whether the named law is selected for this invocation.
func Off(law string) bool {
	return law != "" && os.Getenv(Env) == law
}
