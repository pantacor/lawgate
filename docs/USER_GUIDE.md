# User Guide

This guide shows how to adopt lawgate in a repository. For the meaning of each
term, see [Concepts](CONCEPTS.md). For a failing gate, see
[Troubleshooting](TROUBLESHOOTING.md).

## Install

lawgate needs Go 1.25 or later. It uses only the standard library.

```bash
go install github.com/pantacor/lawgate/cmd/lawgate@v0.1.1
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
| `owner` | The required code owner, such as `@name` or `@org/team`. It turns on the CODEOWNERS check and needs `platform`. The approval check needs a user (`@name`): only a user can approve. |
| `platform` | `github` or `gitlab`. |
| `require_approval` | `true` turns on the approval check of `rule-changes` (see [The owner's approval](#the-owners-approval)). Default `false`. Needs `owner` and `platform`. |
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

A rule file is a file that declares or enforces a rule. The owner must
approve every change to one. lawgate finds the rule files, checks that
CODEOWNERS names the owner for each, and, with `require_approval` on,
checks in CI that the owner approved the pull request or merge request
that changes one.

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

CODEOWNERS is the list of rule files. lawgate does not rely on the
platform's code-owner review, branch protection or required checks: on the
free plans of GitHub and GitLab they are paid features. Approval is the
owner's approval on the pull request or merge request, and the CI job
checks it when `require_approval` is on (see
[The owner's approval](#the-owners-approval)).

### New rule files

In CI, `check-codeowners --base REF` also checks the new rule files against
the CODEOWNERS of the base branch. A new rule file must already be covered
there. There are two ways:

- Keep a fallback. With `* @owner` as the first line, a new file is covered
  at once.
- Without a fallback, merge the CODEOWNERS entry first and the rule file
  after it.

The first change that adds lawgate has no base to compare with. The owner
reviews and merges it.

### The audit of rule changes

```bash
lawgate rule-changes BASE HEAD
```

This lists each commit between the merge base and `HEAD` that changed a rule
file, with the files it changed. A commit can carry a
`Rule-change-approved-by:` trailer. The audit prints it as a note. The
trailer does not replace the owner's approval on the pull request or merge
request.

### The owner's approval

```json
{ "owner": "@name", "platform": "github", "require_approval": true }
```

With `"require_approval": true` in `lawgate.json` at BASE, `lawgate
rule-changes BASE HEAD` also asks the platform's API, in CI, whether the
owner approved the change. It keeps the audit output above. The check is
off by default: then the command prints only the audit and needs no token
or CI variable. `require_approval` is read at BASE, so a change cannot
switch it off for itself, and the change that switches it on is judged
without it: it takes effect from the next change. `--require-approval`
turns the check on for one run whatever `lawgate.json` says.

- No commit in BASE..HEAD changes a rule file: it passes and says so.
- The owner is `owner` in `lawgate.json` at BASE, so a change cannot name
  its own approver. It must be a user. A team or group (`@org/team`) cannot
  approve a pull request or merge request, and is refused.
- **A pull request or merge request the owner authored counts as
  approved** (owner decision 2026-11-24: GitHub does not let an author
  approve their own pull request). The verdict says so: `authored by
  @owner: counts as approved`.
- **GitHub pull request** (events `pull_request` and
  `pull_request_review`): the owner's latest review that decides must be
  `APPROVED` and be on the pull request's head commit. A later request for
  changes or a dismissal wins. A comment decides nothing. HEAD must be the
  pull request's head.
- **GitLab merge request**: the owner must be listed in the merge request's
  approvals. GitLab Free keeps an approval when new commits are pushed, so
  the owner's latest "approved this merge request" note must also be newer
  than the latest push (the newest diff version), and not followed by an
  "unapproved" note. HEAD must be the merge request's latest version.
- **Push to the default branch** (GitHub `push`; GitLab
  `CI_PIPELINE_SOURCE=push` on `CI_DEFAULT_BRANCH`): every rule-file commit
  must come from a merged pull request or merge request that the owner
  approved at its last head, or authored. A rule-file commit pushed without
  one fails, and the message names it. A commit the owner wrote and pushed
  directly fails too: it still needs a pull request or merge request.
- **Merge queue** (`merge_group`): no approval is read. Each pull request
  was judged before it joined the queue, and the push judges what lands.
- Outside CI, or without the token, it fails and names the missing
  variable. There is no local escape.

A failure says what is missing and how to fix it:

```
rule files changed (LAWS.lock, LAWS.md): approval by @owner on 1a2b3c4 is missing (@owner approved 0f9e8d7, an older commit); approve the pull request at its head; the review runs the check again
```

| Platform | Variables it reads |
| --- | --- |
| GitHub Actions | `GITHUB_EVENT_NAME`, `GITHUB_EVENT_PATH`, `GITHUB_REPOSITORY`, `GITHUB_API_URL`, `GITHUB_TOKEN` |
| GitLab CI | `CI_MERGE_REQUEST_IID`, or `CI_PIPELINE_SOURCE`, `CI_COMMIT_BRANCH` and `CI_DEFAULT_BRANCH` on a push; `CI_PROJECT_ID`, `CI_API_V4_URL`, `LAWGATE_API_TOKEN` |

GitHub Actions sets all of them except the token: pass
`GITHUB_TOKEN: ${{ github.token }}` to the step, with `permissions:
pull-requests: read`. On GitLab, add a masked CI/CD variable
`LAWGATE_API_TOKEN` holding a token with the `read_api` scope that can read
the project. `CI_JOB_TOKEN` cannot be used: it cannot read a merge
request's approvals, notes or versions. Use a project access token with the
Reporter role where your plan offers them, else the personal access token
of a dedicated user who is a Reporter of this project only. Do not mark the
variable protected: merge request pipelines run on unprotected branches.

What the check buys depends on the plan:

- **GitLab Free.** Turn on "Pipelines must succeed" (Settings > Merge
  requests). A missing approval fails the `review-rules` job and blocks the
  merge. Approving starts no pipeline: after the owner approves, rerun the
  job or run a new pipeline for the merge request.
- **GitHub Free, private repository.** Nothing can block a merge. A missing
  approval turns the pull request check red, and the run on the push to the
  default branch names any rule-file commit that landed without it. That is
  detection, not prevention. An approval or a dismissal runs the check
  again (`pull_request_review`).
- Where rulesets or protected branches with required checks are available,
  require the two jobs as well.

Two limits remain. While agents push and open pull requests or merge
requests with the owner's credentials, their changes look authored by the
owner and pass the check. Run agents under their own account to close
that. And on the free plans nothing stops a change from editing the CI
file to drop the check from its own run: the CI file is a rule file, so
the audit lists that change, but only a person reading it sees it.

The HTTP calls time out after 30 seconds, follow `Link` (GitHub) and
`X-Next-Page` (GitLab) pagination on the API's host only, and never print
the token.

## CI templates

`lawgate init` writes the template for your platform. Both templates run two
jobs:

| Job | Commands |
| --- | --- |
| `review-rules` | `lawgate check-codeowners --base BASE`, then `lawgate rule-changes BASE HEAD` (which checks the owner's approval when `require_approval` is on) |
| `check-laws` | `lawgate check` |

Both install lawgate with `go install` at a pinned version. Change the
version in the file when you upgrade.

**GitHub Actions.** The workflow runs on `pull_request`, on
`pull_request_review` (submitted and dismissed, so an approval runs the
check again), on `merge_group`, and on pushes to the default branch (set
its name under `push`). On a pull request it checks the merge candidate.
In a merge queue the approval is not read again: the pull request was
judged before it joined the queue, and the push judges what lands. A
review does not run `check-laws` again. The workflow has `permissions:
contents: read, pull-requests: read` and passes `GITHUB_TOKEN` to
`rule-changes`; both are harmless while `require_approval` is off. Every action is pinned to a commit SHA, with the release
tag in a comment. The `review-rules` job needs the full history, so its
checkout uses `fetch-depth: 0`. The `check-laws` job adds the summary to
the job page and uploads the report directory.

**GitLab CI.** Add the jobs to `.gitlab-ci.yml`. They run in merge request
pipelines and in push pipelines of the default branch, with BASE the merge
request's diff base (or the commit before the push) and HEAD the pipeline's
commit. They use only GitLab Free: merge request pipelines test the source
branch head, because merged-results pipelines are a paid feature. With
`require_approval` on, turn on "Pipelines must succeed" and add the
`LAWGATE_API_TOKEN` variable (see
[The owner's approval](#the-owners-approval)); with it off, no token is
needed. For Node modules, use a job
image that has Go and Node.js. The CODEOWNERS fallback sits in a
`[Law gate]` section: keep it.

The templates are in [`templates/`](../templates/).
