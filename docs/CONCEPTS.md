# Concepts

This page explains the words lawgate uses. Each one has a short example. Most
examples come from the [example module](../example/), which lawgate's own gate
checks.

## Law

A law is one rule the code must keep. It is a section in a `LAWS.md` file. The
heading is the law's name in kebab-case. The first sentence says what the law
forbids.

```markdown
## rejects-empty-payload

Processing a job with an empty payload fails before writing a completed result.

- Source: `../README.md`, *Example adopting module* (agreed 2026-11-20).
- Enforced by: test `TestRejectsEmptyPayload`
- Status: agreed 2026-11-20
```

People write laws. An agent or a developer reads them before changing the
module and runs the gate before committing. Only the owner changes a law.

## Source

A source says where a law comes from. A law is never invented: it comes from a
document or from a decision the owner made on a date. The gate accepts three
kinds of source:

- a path to another file in the repository, which must exist;
- a URL;
- the words `owner decision` followed by a date, as in `owner decision 2026-11-20`.

A path is read from the directory of the law document. A law cannot cite its
own document, and it cannot cite `go.mod`.

```markdown
- Source: docs/design.md (agreed 2026-11-20)
- Source: owner decision 2026-11-20
```

## Enforcer

An enforcer is the thing that checks a law. The `Enforced by:` line names it.

| Enforcer | Meaning |
| --- | --- |
| ``machine `worker` `` | The machine `worker` declares a law of this name and reports a verdict for it. |
| ``test `TestName` `` | A test of this name ran to completion and names the law in its doc comment or body. |
| ``script `path` `` | The script at this path exits 0. It is run from the repository root. |
| `not yet. <why>` | The law is agreed but nothing checks it yet. The report shows it as pending, with the reason. |
| `proposed` | The law waits for the owner. |

A law can name more than one enforcer. A Go test name can be qualified as
`import/path/TestName` when two packages have a test of the same name.

## Status

The status is `agreed <YYYY-MM-DD>` or `proposed`. An agreed law must have an
enforcer or a recorded gap. A law that is agreed with `Enforced by: proposed`
is a problem.

## Machine

A machine is the control flow of a worker, declared as data: its states, the
steps each state runs, the edges between states and what happens on each kind
of failure. A machine runs nothing. It sits beside the real code and the gate
checks that the two agree.

```go
func loadState() machine.State {
	sites := []machine.Site{{Tag: "1", Kind: machine.SiteFault}}
	steps := []machine.Step{{ID: "load-job", Kind: "store", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: "validate", Kind: machine.EdgeNext, Name: "Validate"}}
	return machine.State{ID: "load", Func: "Process", Steps: steps, Edges: edges}
}
```

A machine law is a Go function over that graph. This one says that every path
validates the job before it stores the result:

```go
Check: func(m *machine.Machine) []string {
	if m.AlwaysBefore("validate-input", "store-done") {
		return nil
	}
	return []string{"step validate-input does not always precede store-done"}
},
```

A step is identified by its state and its id. A step id may repeat across
states, because a product names a step after the work it does: Fleet's
appliance deployer runs `close-open-children` in eleven states, and those
names come from the Hub API, so they are never renamed. `machine.Validate`
refuses a step id only when one state declares it twice. The step helpers
therefore mean every occurrence of an id: `AlwaysBefore(a, b)` holds when
every occurrence of b is preceded, on every path from the entry state, by
some occurrence of a (an occurrence never precedes itself), and
`SomePathBefore(a, b)` holds when some occurrence of a can run before some
occurrence of b. To mean one occurrence, name its state with a
`machine.StepRef`:

```go
check := machine.StepRef{Step: "check-balance"}
closing := machine.StepRef{State: "refund", Step: "close-account"}
ok := m.AlwaysBeforeRef(check, closing)
```

`m.StepIn(state, id)` and `m.StatesWithStep(id)` look steps up the same way.

### Sites and drift

A site is a place in the code where a failure is reported or a write is made.
The code marks it with a tag in a log or error text, such as `WORKER_JOB 2`.
Package `resolve` reads the Go source and checks three things: every tag in
the code is claimed by a declared site, every declared site is in the code,
and the kind a site declares matches the call the code makes. A tag in a
comment does not count.

When the code and the machine stop agreeing, that is drift. The gate reports
it. A branch with no tagged call is invisible to the machine: cover it with a
test or with a kernel.

## Witness

A witness shows that a machine law can fail. It is a small change to a copy of
the machine that must break the law.

```go
Witness: func(m *machine.Machine) {
	bypass := machine.Edge{On: "skip", To: "complete", Kind: machine.EdgeNext}
	m.States[0].Edges = append(m.States[0].Edges, bypass)
},
```

This witness adds an edge that skips validation. The law
`validate-before-store` must then report a violation. `machine.CheckLaws`
refuses a law with no witness, a witness that does not change the machine, and
a witness that does not break the law.

## Typed states and the run-time trace

Package `gen` writes one Go type for each state. The methods of a type are the
edges the state declares. Worker code that moves along an undeclared edge does
not compile.

Package `run` records each move of one pass. It refuses a move that leaves a
state the pass is not in, and a move over an edge taken more times than its
limit allows.

```go
pass := run.New(worker.Machine())
err := pass.Enter("load")                  // the pass starts in the entry state
err = pass.Go("load", "Validate", "validate")
err = pass.Go("load", "Validate", "validate")
// refused: move leaves "load" but the pass of worker is in "validate"
```

`run.IsPath` replays a recorded trace against the machine with the same rules.

## Kernel and prove

A kernel is a small pure function where a bug matters: a revision number, an
access filter, a token check. Its inputs are bounded. Package `prove` runs
every law on every input inside the bounds, so within the bounds a pass is a
proof.

```go
// nextRevision is the kernel.
func nextRevision(current int) int { return current + 1 }

func TestRevisionsOnlyGrow(t *testing.T) {
	grows := prove.Law[int]{
		Name: "revisions-only-grow",
		Holds: func(current int) error {
			if nextRevision(current) > current {
				return nil
			}
			return fmt.Errorf("revision %d did not grow", current)
		},
	}
	prove.Check(t, prove.Ints(0, 1000), grows)
}
```

`prove.Check` fails the test when a law is broken, when the list of laws is
empty and when the input space is empty. Proving nothing is not a pass.
`prove.Of`, `prove.Concat` and `prove.Product` build larger input spaces.

## Bite

A law bites when its enforcer fails once the protected behaviour is gone. A
test that passes whatever the code does protects nothing, so the gate checks
this for every agreed law. Production code carries no test switch: the gate
breaks a copy of the code itself.

A machine law bites through its witness. A law enforced by a Go test bites
through its subject.

## Subject

The subject is the function a law protects. The law names it in `LAWS.md`:

```markdown
- Subject: github.com/pantacor/lawgate/example.hasPayload
```

```go
func hasPayload(job Job) bool {
	return strings.TrimSpace(job.Payload) != ""
}
```

Write `<import path>.<Func>` or `<import path>.<Type>.<Method>`. An agreed
law enforced by a Go test has a subject, or records the gap with
`Subject: not yet. <why>` and is reported as pending.

## Mutation

A mutant is a copy of the subject with one condition changed. The gate
makes the mutants. The author does not choose them:

- each `if`, `for` and `switch` case condition is negated;
- each comparison is flipped (`<` and `<=`, `>` and `>=`, `==` and `!=`);
- the condition that guards a `return` of an error becomes `false`.

For `hasPayload` the gate turns `!=` into `==`. The law's test must then
fail. The gate applies each mutant with `go test -overlay`, so the files in
the repository do not change. A mutant that the test does not catch
survives, and a surviving mutant fails the `laws-bite` verdict with the
subject line and the mutation.

## Proof and lock

The proof of a law is its enforcing tests and every test function they call.
`LAWS.lock` records, for each law, a fingerprint of the proof and a
fingerprint of the subject. `lawgate lock` writes the file. `lawgate check`
fails when a fingerprint differs from the lock.

The lock is a rule file. A change to a law's test or subject changes the
lock, and the owner reviews the lock. So a law's test cannot vouch for
itself: someone who weakens the code cannot also rewrite the test and merge
both without the owner.

## Base replay

In CI the gate runs `lawgate check --base <sha>`. For every law agreed at
the base commit, it takes the base version of the enforcing tests, puts them
over the candidate code and runs them. The proof the owner approved must
still pass against the change. A failure names the law. A law the change
removes or renames also fails, until the owner updates the lock.

```
base commit:  proof (approved)  +  code
candidate:    proof (edited?)   +  code (changed)
replay:       proof from base   +  code from candidate   must pass
```

## Node tests and scripts

Node tests and scripts have no subject and no mutation yet. The gate reruns
them with the `LAWGATE_BITE` environment variable set to the law's name,
and the enforcer must fail. A control rerun with an unrelated name must
pass. This is a known gap: the enforcer itself decides what to switch off.

## Rule file

A rule file is a file that declares or enforces a rule. Changing one can
weaken the gate, so the owner must review it. Rule files include:

- `lawgate.json`, the readability baseline and every law document;
- CODEOWNERS and the CI files;
- the paths listed in `gate_files`;
- the Go packages that declare machines or laws, the tests that enforce laws,
  and every package in the repository they import.

`lawgate rule-files` prints the list.

## Ownership closure

The closure is everything a rule file reaches. If a law test calls a helper in
another package, a change to that helper can make the test pass for the wrong
reason. So lawgate follows the imports from each enforcing test and each rule
declaration, and protects every package it reaches in full: its Go files, its
tests, its `testdata/`, its embedded files and its module files.

```
example/worker_test.go            enforces rejects-empty-payload
  └─ example/worker.go            same package, protected
       └─ lawgate/gen             imported, protected
```

`lawgate codeowners` prints one exact CODEOWNERS entry for each file in the
closure. `lawgate check-codeowners` fails when the owner does not cover one.

## Owner

The owner is the person or team that approves rule changes. It is set in
`lawgate.json`, for example `"owner": "@your-org/law-owners"`. Approval is the
platform's code-owner review, required by branch protection. In CI the owner
is read from the base branch, so a change cannot name its own reviewer.

## Fail closed

When the gate cannot check something, it fails. It does not skip the check and
report green. These all fail the gate:

- a module listed in `lawgate.json` that is missing;
- a machine runner that crashes or writes no report;
- a report with zero verdicts;
- an enforcing test that did not run to completion;
- a generated file that no `generate_check` target covers;
- a rule file that cannot be read or parsed.

The summary shows such a case as an infrastructure failure, apart from broken
laws.

## Pending

Pending means a law waits on a decision. It is not a pass and not a failure.
A law is pending when it records a gap with `Enforced by: not yet. <why>` or
`Subject: not yet. <why>` or `Bites: not yet. <why>`. Reports list pending items under their own heading,
each with its reason.
