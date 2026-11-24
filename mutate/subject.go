// Package mutate breaks a law's subject on purpose, so the gate, not the
// author, decides what a law's tests must catch. A subject is the Go function
// a law protects. Each mutant changes one guard condition of that function
// (negated, a comparison flipped, or a guarded return disabled); the law's
// enforcing tests must fail for every one. A test that still passes against a
// mutant does not depend on the guard it claims to prove.
//
// A guard is an if, for or expression-less switch case condition, a
// variable assigned or declared with a condition, or a condition a function
// returns. Conditions are recognised by their form (a comparison, && / ||,
// or !), since the package reads syntax, not types.
//
// Limit: a guard that only passes an error up unchanged is not mutated, as
// error propagation is not law behaviour. That is an if with no else whose
// condition is `err != nil` (any name ending in err) and whose body is one
// return of zero values followed by that error, bare, wrapped by
// fmt.Errorf with %w, or joined by errors.Join. Such guards are counted
// (Target.ErrorPropagations) so a verdict can say how many it left. Any
// other use of the error in a guard (err != nil && more, errors.Is, a
// branch that returns a value or changes state) is still mutated.
//
// Mutants are applied through `go test -overlay`: the repository's files are
// never written.
package mutate

import (
	"fmt"
	"go/token"
	"strings"
)

// Subject names a function, or a method of a named type, in a Go package.
type Subject struct {
	ImportPath string
	Receiver   string // the method's type name; empty for a function
	Func       string
}

// ParseSubject reads `<import path>.<Func>` or `<import path>.<Type>.<Method>`.
// The import path may itself contain dots (example.org/shop.Pay): only the
// text after its last slash is split.
func ParseSubject(reference string) (Subject, error) {
	reference = strings.Trim(strings.TrimSpace(reference), "`")
	slash := strings.LastIndex(reference, "/")
	parts := strings.Split(reference[slash+1:], ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Subject{}, invalidSubject(reference)
	}
	subject := Subject{ImportPath: reference[:slash+1] + parts[0], Func: parts[len(parts)-1]}
	if len(parts) == 3 {
		subject.Receiver = parts[1]
	}
	if !subject.valid() {
		return Subject{}, invalidSubject(reference)
	}
	return subject, nil
}

func (subject Subject) valid() bool {
	identifiers := token.IsIdentifier(subject.Func) && validPackage(subject.ImportPath)
	if subject.Receiver == "" {
		return identifiers
	}
	return identifiers && token.IsIdentifier(subject.Receiver)
}

func validPackage(path string) bool {
	last := path[strings.LastIndex(path, "/")+1:]
	return last != "" && !strings.ContainsAny(path, " \t`")
}

func invalidSubject(reference string) error {
	return fmt.Errorf("subject %q is not <import path>.<Func> or <import path>.<Type>.<Method>", reference)
}

// String is the subject as LAWS.md names it.
func (subject Subject) String() string {
	if subject.Receiver == "" {
		return subject.ImportPath + "." + subject.Func
	}
	return subject.ImportPath + "." + subject.Receiver + "." + subject.Func
}
