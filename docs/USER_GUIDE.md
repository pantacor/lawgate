# User Guide

This guide shows how to adopt lawgate in a repository. For the meaning of each
term, see [Concepts](CONCEPTS.md). For a failing gate, see
[Troubleshooting](TROUBLESHOOTING.md).

## Install

lawgate needs Go 1.25 or later. It uses only the standard library.

```bash
go install github.com/pantacor/lawgate/cmd/lawgate@latest
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

A fourth field is optional: `- Bites: not yet. <why>` records that a test or
script enforcer has no breaking case yet.

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

### Generated state types

Package `gen` can write a Go type for each state, so that code can only move
along declared edges. Generated files must stay fresh. Give the module a
`generate_check` command that calls `gen.ReportCheck`. The example uses:

```json
"generate_check": ["go", "run", "./cmd/genstates", "-check"]
```

## Make a law bite

Every agreed law with a test or script enforcer must show that it can fail.
For a Go test, add a `bite.Off` call with the law's name to the non-test code
that the test reaches:

```go
import "github.com/pantacor/lawgate/bite"

func acceptsPayload(payload string) bool {
	return bite.Off("rejects-empty-payload") || payload != ""
}
```

The test asserts that an empty payload is rejected. `bite.Off` returns false
in an ordinary build, so the hook costs nothing in production.

The gate reruns the test in a build with the `lawgate_bite` tag. First it
selects a law that does not exist, and the test must pass. Then it sets
`LAWGATE_BITE` to the law's name, and the test must fail.

For Node tests and scripts, read the `LAWGATE_BITE` environment variable
yourself and compare it with the exact law name:

- A Node test must fail when its law is selected.
- A script must print the line `lawgate: rejects <law>` and exit 1. Any other
  nonzero exit is an infrastructure failure.

If you cannot write the hook yet, record the gap. The law is then reported as
pending:

```markdown
- Bites: not yet. The store has no fake that can drop a write.
```

Do not set `LAWGATE_BITE` in your shell. The `lawgate` command refuses to
start while it is set.

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
