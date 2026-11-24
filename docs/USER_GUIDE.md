# User Guide

This guide shows how to adopt lawgate in a repository. For the meaning of each
term, see [Concepts](CONCEPTS.md). For a failing gate, see
[Troubleshooting](TROUBLESHOOTING.md).

## Install

lawgate needs Go 1.25 or later. It uses only the standard library.

```bash
go install github.com/pantacor/lawgate/cmd/lawgate@v0.1.0
```

Pin a release, and use the same release in CI. From a checkout of lawgate, run
`go run ./cmd/lawgate` instead.

## Start with `lawgate init`

Run this at the root of your repository:

```bash
lawgate init --platform github --owner @your-org/law-owners
```

`--owner` is required. `--platform` is `github` (the default) or `gitlab`.
`init` finds one module for each `go.mod` and writes four files:

| File | Content |
| --- | --- |
| `lawgate.json` | The modules it found, the owner and the platform. |
| `.lawgate-readability` | An empty readability baseline. |
| `.github/CODEOWNERS` (GitLab: `CODEOWNERS`) | One entry for each rule file. |
| `.github/workflows/lawgate.yml` (GitLab: `.gitlab-ci.yml`) | The CI jobs. |

`init` refuses to overwrite a file that exists. Add `--fallback` to start
CODEOWNERS with `* @owner`, so a new rule file is covered before the list
names it.

After `init`, open `lawgate.json`. Check each module's `laws` path, and add
your Node modules by hand.

## `lawgate.json`

`lawgate.json` lists what the gate must check. An unknown key is an error.
Paths use `/` and are relative to the repository root.

```json
{
  "owner": "@your-org/law-owners",
  "platform": "github",
  "modules": [
    {
      "path": ".",
      "language": "go",
      "laws": "LAWS.md",
      "machines": true
    },
    {
      "path": "chat",
      "language": "node",
      "laws": "chat/LAWS.md",
      "machines": false
    }
  ],
  "readability": { "baseline": ".lawgate-readability" },
  "gate_files": ["scripts/test-laws.sh"]
}
```

### Top-level fields

| Field | Meaning |
| --- | --- |
| `modules` | Required. The modules to check. It cannot be empty. |
| `owner` | The required code owner, such as `@name` or `@org/team`. It turns on the CODEOWNERS check and needs `platform`. |
| `platform` | `github` or `gitlab`. |
| `product` | A name shown in the HTML report. |
| `readability` | The readability check. `baseline` is required inside it. `include` and `exclude` are optional lists of globs. |
| `gate_files` | Extra paths the owner must review, such as a script that wraps the gate. |
| `rule_packages` | Go packages of your own whose declarations are rules, each as `{"path", "names"}`. |
| `outside_includes` | Absolute directories outside the repository that the owner approves for cgo headers. |

### Module fields

| Field | Meaning |
| --- | --- |
| `path` | Required. The module directory. Use `"."` for the root. |
| `language` | Required. `go` or `node`. |
| `laws` | Required. The path to the module's law document. |
| `machines` | Required. `true` when the module declares machines. |
| `machine_command` | The command that checks the machines. The Go default is `["go","run","./cmd/machine"]`. A Node module with machines must set it. |
| `machine_build` | Needed only when a Go machine command is a script or a Makefile target. It tells lawgate which build the runner uses. |
| `test_command` | Replaces the test command. The Go default is `["go","test","-json","-count=1","./..."]`. The Node default is `["node","--test","--test-reporter=tap"]`. |
| `required_tests` | Extra tests that must run to completion. |
| `shared_laws` | Law documents shared by several modules. |
| `generate_check` | The command that checks generated Go files. Required when the module has a file with a `// Code generated ... DO NOT EDIT.` header. |
| `timeout_seconds` | The deadline for each command of the module, from 1 to 86400. The default is 300. |

The [README](../README.md) has the full rules for Go command flags, readability
globs and cgo headers.

## Write `LAWS.md`

Each module has one law document. A law is a `## ` or `### ` heading whose
text is a kebab-case name. Any other heading groups the laws below it.

```markdown
# Worker laws

## Jobs

### rejects-empty-payload

Processing a job with an empty payload fails before writing a completed result.

- Source: docs/design.md (agreed 2026-11-20)
- Enforced by: test `TestRejectsEmptyPayload`
- Status: agreed 2026-11-20
```

The three fields are required:

- **Source**: a path to a file in the repository, a URL, or
  `owner decision YYYY-MM-DD`. A path is read from the directory of the law
  document and must exist.
- **Enforced by**: ``machine `name` ``, ``test `TestName` ``,
  ``script `path` ``, `not yet. <why>` or `proposed`. List several enforcers
  on one line, separated by commas.
- **Status**: `agreed YYYY-MM-DD` or `proposed`.

A law enforced by a Go test also needs `- Subject: <import path>.<Func>`, the
function the law protects, or `- Subject: not yet. <why>`. For a Node test or
a script, `- Bites: not yet. <why>` records that the enforcer has no breaking
case yet. See [Make a law bite](#make-a-law-bite).

Three rules to remember:

- A law has one section across the module's own document and its shared ones.
- A name that starts with `lawgate/` is refused. lawgate keeps that prefix.
- A Go test must name its law in its doc comment or in a string in its body.

```go
// TestRejectsEmptyPayload enforces rejects-empty-payload.
func TestRejectsEmptyPayload(t *testing.T) {
	// ...
}
```

A script enforcer is run from the repository root. It passes when it exits 0.

## Declare a machine and its runner

Use a machine when a worker has states, retries and failure paths that a
single test cannot cover. A module with `"machines": true` needs three things.

**1. A machine declaration.** Declare the states, steps, sites and edges in
Go, and give each machine law a `Check` and a `Witness`. See
[`example/machine.go`](../example/machine.go).

A catch rule sends every site of one kind to a state. A catch with no
target carries on instead: the pass stays where it is, and the note says
where the error is logged.

```go
catches := []machine.Catch{
	{On: machine.SiteFault, To: "retry", Note: "the job retries on the ladder"},
	{On: machine.SiteWarn, Note: "a warning in the job's log; the job carries on"},
}
```

A carry-on catch is not an edge, takes no limit, and records no move at run
time; the runner's outline and brief show it as "carries on".

**Shared worker laws.** Package `laws` has the laws worker products share,
each with its check and a witness that breaks it, under the names Fleet and
the Pantavisor AI stack use in their `LAWS.md`. Product facts are
parameters:

| Constructor | Law |
| --- | --- |
| `laws.NothingSilent()` | `nothing-silent`: no site is `silent`. |
| `laws.EveryFailureSaysWhy(kinds...)` | `every-failure-says-why`: every site of those kinds (default fail, fault, warn) has an `Effect`. |
| `laws.ReachesOneOf(name, doc, targets...)` | every working state can reach one of the target states. |
| `laws.NoStuck(name, doc, statuses...)` | every state that is not terminal, waits included, can reach a terminal state (showing one of the statuses). |
| `laws.WaitsEnd(name, doc, allow...)` | a wait entered on a `no limit` guard names what wakes it. |
| `laws.DoneMeansStored(done, label)` | every way into the done state leaves a state with a write site labelled `label`. |
| `laws.OneShotFailsOnFault(label, failed, kinds...)` | on a machine labelled `label`, every caught fault ends in the terminal failed state, and nothing waits. |

```go
shared := []machine.Law{
	laws.NothingSilent(),
	laws.EveryFailureSaysWhy(),
	laws.DoneMeansStored("done", "stores-done"),
}
```

**2. A vocabulary.** It tells lawgate how your code marks a site: the pattern
of a tag, and which call means which kind of site.

```go
func Vocabulary() resolve.Vocabulary {
	calls := map[string]machine.SiteKind{
		"fault":     machine.SiteFault,
		"failJob":   machine.SiteFail,
		"storeDone": machine.SiteWrite,
	}
	return resolve.Vocabulary{
		TagPattern: regexp.MustCompile(`^(WORKER_JOB \d+)`),
		Calls:      calls,
	}
}
```

**3. A runner.** Put it in `./cmd/machine`:

```go
package main

import (
	worker "github.com/pantacor/lawgate/example"
	"github.com/pantacor/lawgate/runner"
)

func main() {
	runner.Main([]runner.Entry{worker.Entry()})
}
```

`runner.Main` reads the source file of each machine, binds it with `resolve`,
checks every law and writes a JUnit report. `lawgate check` runs the machine
command from the module directory and adds `-laws -junit FILE`.

You can run the runner yourself. Without `-junit` it prints a Markdown
summary:

```bash
go run ./cmd/machine -laws
```

The exit code means:

| Code | Meaning |
| --- | --- |
| `0` | Every machine resolved and every law holds. |
| `1` | A law is broken, or the code and the machine have drifted. |
| other | A usage, flag or I/O error. The gate records an infrastructure failure. |

The runner also shows the machines it declares. Each run does one thing;
`-machine NAME` narrows it to one machine:

| Flags | Output |
| --- | --- |
| (none) | The outline of each machine: catches, states, steps, sites with their lines, edges, findings, and a summary line. `-color` colours it for a terminal. |
| `-list` | One line per machine: name, source, state count and law count. |
| `-json` | One machine for a viewer: the declaration with each site's line, `summary`, `drift` (file, line, message), `laws` (verdicts) and the source `lines`. |
| `-brief STATE` | The Markdown brief an agent needs to change one state: how it is entered, the lines its steps own, the edges and catches it must keep. |
| `-index FILE` | The machine index `lawgate review` reads. |

`-json` and `-brief` show one machine, so with several declared they need
`-machine`. These views exit `0` when shown, `1` when shown but the
declaration has drifted from its source (the drift is on standard error), and
`2` on a usage or I/O error.

```bash
go run ./cmd/machine -list
go run ./cmd/machine -machine worker -brief validate
```

### Generated state types

Package `gen` can write a Go type for each state, so that code can only move
along declared edges. Generated files must stay fresh. Give the module a
`generate_check` command that calls `gen.ReportCheck`. The example uses:

```json
"generate_check": ["go", "run", "./cmd/genstates", "-check"]
```

## Make a law bite

Every agreed law must show that it can fail. Your production code needs no
hook for this.

### Go tests: name the subject

Add a `Subject:` line with the function the law protects:

```markdown
### rejects-empty-payload

Processing a job with an empty payload fails before writing a result.

- Source: owner decision 2026-11-20
- Enforced by: test `TestRejectsEmptyPayload`
- Subject: example.com/shop/worker.hasPayload
- Status: agreed 2026-11-20
```

```go
func hasPayload(job Job) bool {
	return strings.TrimSpace(job.Payload) != ""
}
```

The gate changes one condition of the subject at a time (here `!=` becomes
`==`) and runs the law's tests against each changed copy. The tests must
fail every time. Your files are not edited: the gate uses `go test -overlay`.

Two tips:

- Keep the subject small. Put the law's guard in its own function. Every
  condition in the subject is mutated, and the law's tests must catch each
  one.
- Test both sides. Assert that the bad input is refused and that a good
  input is accepted. A guard that refuses everything is also a mutant.

If the law has no subject yet, record the gap. The law is then reported as
pending:

```markdown
- Subject: not yet. The check lives in a generated file.
```

### Lock the proofs

```bash
lawgate lock
git add LAWS.lock
```

`LAWS.lock` holds a fingerprint of each law's tests and of its subject.
`lawgate check` fails when they differ:

```
proof of rejects-empty-payload changed: update LAWS.lock, which needs the owner's review
```

When you change a law's test or its subject on purpose, run `lawgate lock`
again and commit the new lock with the change. The lock is a rule file, so
the owner must approve it. The default path is `LAWS.lock` at the repository
root. Set `"lock"` in `lawgate.json` to move it.

### Replay the approved proofs

In CI the templates run `lawgate check --base <sha>` with the merge base.
The gate takes the law tests as they were at the base and runs them against
your changed code. They must pass. Try it before you push:

```bash
lawgate check --base origin/main
```

### Node tests and scripts

Node tests and scripts do not have subjects yet. Read the `LAWGATE_BITE`
environment variable yourself and compare it with the exact law name:

- A Node test must fail when its law is selected.
- A script must print the line `lawgate: rejects <law>` and exit 1. Any other
  nonzero exit is an infrastructure failure.

If you cannot write that case yet, record the gap. The law is then reported
as pending:

```markdown
- Bites: not yet. The store has no fake that can drop a write.
```

Do not set `LAWGATE_BITE` in your shell. The `lawgate` command refuses to
start while it is set. Go code never reads it.

## Run the gate

```bash
lawgate check
```

`check` prints the path of a fresh run directory under `laws-report`. Use
`--out DIR` to choose another parent directory and `--root DIR` to check
another repository. The run directory holds:

| File | Use |
| --- | --- |
| `laws-summary.md` | A Markdown summary for a job page. |
| `laws-report.html` | One HTML page with every verdict. |
| `001.xml`, `002.xml`, ... | JUnit files, one for each module and one for the repository-wide checks. |

The command exits 1 when a law is broken or a check could not run. Add
`laws-report/` to your `.gitignore`.

Tell contributors, and agents in `AGENTS.md`, to read the module's `LAWS.md`
before a change and to run the gate before each commit.

## Readability

The readability check applies five rules to Go code:

| Rule | Limit |
| --- | --- |
| `nesting` | No statement more than 2 block levels deep inside a function. A loop's single leading guard does not count. |
| `inline-nesting` | No struct or map literal written inline inside another one. |
| `func-length` | No function body longer than 40 lines. |
| `condition` | No condition with 3 or more `&&` or `||` operands. Give it a name. |
| `bool-param` | No `bool` parameter. |

Existing code rarely passes on the first day. The baseline lets you start
from where you are:

```bash
lawgate readability-baseline
```

This writes one `count path` line for each file with findings. From then on
the count of a file can only go down. A file whose count grows fails the
gate, and so does a new file with any finding. When a count goes down, the
summary says the baseline can shrink.

An empty baseline allows zero findings. The baseline is a rule file, so the
owner reviews every change to it.

## CODEOWNERS and rule changes

A rule file is a file that declares or enforces a rule. The owner must review
every change to one. lawgate finds the rule files and checks that CODEOWNERS
names the owner for each.

```bash
lawgate rule-files                    # list the rule files
lawgate codeowners --fallback         # print the CODEOWNERS entries
lawgate check-codeowners              # fail when the owner misses a rule file
```

When you add a law test or a machine, the list of rule files changes.
Regenerate the entries in the same change:

```bash
lawgate codeowners --fallback > .github/CODEOWNERS
```

Then turn on two settings on the protected branch of your platform:

1. Require review from code owners.
2. Require the two CI jobs.

Approval is the platform review. lawgate does not read approvals itself.

### New rule files

In CI, `check-codeowners --base REF` also checks the new rule files against
the CODEOWNERS of the base branch. A new rule file must already be covered
there. There are two ways:

- Keep a fallback. With `* @owner` as the first line, a new file is covered
  at once.
- Without a fallback, merge the CODEOWNERS entry first and the rule file
  after it.

The first change that adds lawgate has no base to compare with. The owner
reviews and merges it, then makes the CI jobs required.

### The audit of rule changes

```bash
lawgate rule-changes BASE HEAD
```

This lists each commit between the merge base and `HEAD` that changed a rule
file, with the files it changed. A commit can carry a
`Rule-change-approved-by:` trailer. The audit prints it as a note. The
trailer does not replace the platform review.

## CI templates

`lawgate init` writes the template for your platform. Both templates run two
jobs:

| Job | Commands |
| --- | --- |
| `review-rules` | `lawgate check-codeowners --base BASE`, then `lawgate rule-changes BASE HEAD` |
| `check-laws` | `lawgate check` |

Both install lawgate with `go install` at a pinned version. Change the
version in the file when you upgrade.

**GitHub Actions.** The workflow runs on `pull_request` and `merge_group`, so
it checks the merge candidate and the merge queue. Every action is pinned to
a commit SHA, with the release tag in a comment. The `review-rules` job needs
the full history, so its checkout uses `fetch-depth: 0`. The `check-laws` job
adds the summary to the job page and uploads the report directory.

**GitLab CI.** Turn on merged-results pipelines: the jobs stop when the
pipeline does not run on the merged result. Inject the jobs from a protected
project with a pipeline execution policy, so a merge request cannot edit
them. For Node modules, use a job image that has Go and Node.js. The
CODEOWNERS fallback sits in a `[Law gate]` section, which must stay
mandatory.

The templates are in [`templates/`](../templates/).
