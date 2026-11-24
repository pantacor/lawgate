// Package bite names the environment variable and the reserved prefix that
// lawgate uses when it reruns a Node test or a script enforcer to see it
// fail. Those enforcers read the variable themselves.
//
// Go code has no switch here. A law enforced by a Go test names its subject
// in LAWS.md, and the gate mutates that function in an overlay instead. A
// hook in production code could be moved to guard something meaningless, so
// this package offers none.
package bite
