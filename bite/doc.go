// Package bite lets a law enforcer demonstrate that it detects a violation.
// Guard the protected behavior with Off, and have the enforcer verify that
// behavior. The enforcer must pass normally and fail when lawgate reruns it
// with the lawgate_bite build tag and LAWGATE_BITE selecting the law.
// The empty selector disables no law. The lawgate/ prefix is reserved for
// framework laws and control runs.
// Ordinary Go builds disable Off. Scripts and non-Go code can read
// the same environment variable directly. Keep the mutation local to the
// process and leave the repository unchanged.
package bite
