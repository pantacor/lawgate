# Troubleshooting

This page lists the most common gate failures. Each entry quotes the message
lawgate prints, says what it means and says what to do.

## Where to look

`lawgate check` prints the path of its run directory and then, on failure:

```
law checks failed; see laws-report/run-1453756126
```

Open `laws-summary.md` in that directory. Broken laws are under **What
broke**. Items that wait on a decision are under **Pending**. For every
verdict in one page, open `laws-report.html`.

The first line of the summary tells you the kind of failure:

| Summary line | Meaning |
| --- | --- |
| `✓ All checks passed.` | Every law holds. |
| `✗ 2 of 14 laws ... broke.` | A law is broken. Fix the code. |
| `✗ Infrastructure failure` | A check could not run. Nothing was proved. Fix the setup first. |
| `… 1 pending.` | A law records a gap. This does not fail the gate. |

## The law document

### `LAWS.md does not exist: the module's laws are not documented`

The `laws` path of a module in `lawgate.json` points to no file. Create the
file or fix the path.

### `law <name> has no "- Source:" line`

The same message exists for `- Enforced by:` and `- Status:`. Every law needs
all three fields. Each field is a list item that starts with `- `.

### `law <name> has invalid Source "...": must cite another repository file (excluding go.mod), a URL, or a dated owner decision (YYYY-MM-DD)`

The source is not one of the accepted forms, or its path does not exist. A
path is read from the directory of the law document, not from the repository
root. For a decision, write the words `owner decision` and the date:

```markdown
- Source: owner decision 2026-11-20
```

### `law <name>: "Status: ..." is neither "agreed <YYYY-MM-DD>" nor "proposed"`

Use one of the two forms. The date must be a real calendar date.

### `law <name> is agreed but nothing enforces it (Enforced by: proposed)`

An agreed law needs an enforcer. Name a machine, a test or a script. If
nothing can check it yet, write `Enforced by: not yet. <why>`. The law is
then reported as pending.

### `law <name> has more than one section`

A law has one section across the module's document and its shared documents.
Remove the copy.

### `law <name> uses the reserved "lawgate/" prefix`

Rename the law. lawgate keeps that prefix for its own control selectors.

## Test enforcers

### `law <name> is enforced by test "TestX", which is not a test function the build compiles in this module`

The name in `LAWS.md` matches no test. Check the spelling. Check that the
file is not excluded by a build tag or a file name suffix such as
`_windows_test.go`.

### `law <name> is enforced by test "TestX", which does not name the law <name> in its doc comment or body`

The test exists but does not say which law it enforces. Add the name:

```go
// TestRejectsEmptyPayload enforces rejects-empty-payload.
```

### `law <name> is enforced by test "TestX", which did not run to completion`

The test was skipped, was filtered out by the test command, or its package
failed to build. Run the module's test command yourself and read its output.
If you set `test_command`, check that it still runs the package of this
test.

### `law <name> is enforced by ambiguous test "TestX"; qualify it as one of: ...`

Two packages have a test of this name. Use the qualified form the message
suggests:

```markdown
- Enforced by: test `example.org/project/workers/TestX`
```

### `test runner produced zero executed test verdicts`

The test command ran no test. With a custom `test_command`, the output must
keep the default format: `go test -json` for Go, TAP for Node.

## Bites

### `mutant survived: <subject> line <n>: <mutation>`

The gate changed one condition of the law's subject and the law's tests
still passed. Check two things:

- Does the test reach that condition? Add the missing case: assert that the
  bad input is refused and that a good input is accepted.
- Is the subject the right function? If it holds conditions the law does not
  care about, move the law's guard into its own function and name that one.

### `law <name> has no subject`

An agreed law enforced by a Go test must name the function it protects:

```markdown
- Subject: <import path>.<Func>
```

Use `<import path>.<Type>.<Method>` for a method. If you cannot name one
yet, record the gap and the law is reported as pending:

```markdown
- Subject: not yet. <why>
```

### `subject <reference> not found`

The import path, type or function name in the `Subject:` line does not match
a function in the module. Check the spelling and that the file is part of
the build.

### `proof of <law> changed: update LAWS.lock, which needs the owner's review`

A test that enforces the law, or a test function it calls, differs from the
fingerprint in `LAWS.lock`. The message says `subject of <law> changed` when
the subject function differs. If the change is intended, run `lawgate lock`
and commit `LAWS.lock` with it. The owner must approve the lock. If it is
not intended, undo the edit.

### `the approved proof of <law> fails against this change`

`lawgate check --base` ran the law's tests as they were at the base commit
against your code, and they failed. Your change breaks the behaviour the
owner approved. Fix the code. If the law itself must change, ask the owner:
the change needs a new proof and a new lock.

### `law <name> was agreed at the base and is missing`

The change removes or renames a law that was agreed at the base commit. Only
the owner can do that, by approving the updated `LAWS.lock`.

### `test TestX passed with LAWGATE_BITE=<law>`

Node tests only. The message can also say `did not fail`. The gate selected
the law and the test still passed, so the test does not check the law. Make
the test fail when `LAWGATE_BITE` equals its law name.

### `test TestX must pass the control with LAWGATE_BITE="lawgate/control/no-such-law"`

Node tests and scripts only. The enforcer failed in the control rerun, where
no law is selected. It must compare the variable with its own law name, not
only check that it is set.

### `script exited without the required line "lawgate: rejects <law>"`

A script that rejects its mutation must print that exact line and exit 1.
Any other nonzero exit counts as an infrastructure failure.

### `LAWGATE_BITE is set in the gate's environment. It is reserved for enforcer reruns.`

Unset the variable in your shell or CI job and run `lawgate` again.

## Machines

### `law <name> is enforced by machine "worker", which is not a registered machine or has no verdicts`

The runner did not report this machine. Add its entry to the list passed to
`runner.Main`, and check the machine name.

### `law <name> (machine worker) has no "## <name>" section`

A machine declares a law that `LAWS.md` does not document. Add the section,
after the owner agrees to the law.

### `Go module "x" declares machines and requires machines: true`

lawgate found a machine declaration in a module listed with
`"machines": false`. Set it to `true` and give the module a runner in
`./cmd/machine`.

### `machine "worker" is missing from runner verdicts`

The source declares the machine but the runner did not check it. Register it
with `runner.Main`.

### `line 34: tag "WORKER_JOB 4" (fail) is not claimed by any site`

The code has a tagged call that the machine does not declare. This is drift.
Add the site to the machine. If the change adds a new failure path, check
that the laws still hold.

### `tag "3" (match "") is not in the code`

The machine declares a site that the code no longer has. Remove the site
from the declaration, or restore the tagged call.

### `site at line 34 is declared fail but the code is fault`

The kind of the declared site does not match the call in the code. Fix the
declaration or the call. The vocabulary maps each call name to a kind.

### `law <name> has no Witness` and `law <name> witness did not break the law`

Every machine law needs a witness: a change to a copy of the machine that
makes the law report a violation. Write one that removes what the law
protects.

### `limit reached: ... taken 4 times, limit 3`

At run time, a pass took an edge or a catch more often than the machine
allows. The loop in the code can run more times than the declaration says.
Fix the code, or ask the owner to change the limit.

## Generated code

### `<file> belongs to no configured module with generate_check`

A Go file has a `// Code generated ... DO NOT EDIT.` header, but its module
has no `generate_check` in `lawgate.json`. Add the command.

### A `generated-is-fresh` verdict fails

The generated file differs from what the generator writes now. Run the
generator again and commit the result. In the example module:

```bash
go generate ./...
```

## Readability

### `path/file.go: 3 findings (grew from baseline 2)`

The file has more findings than the baseline allows. Fix the new finding.
The summary lists each one, for example
`checkCommand nested 3 levels deep (max 2)`.

### `path/file.go: 1 findings (new file, baseline 0)`

A file that is not in the baseline has a finding. New code must pass every
rule. See the rules in the [User Guide](USER_GUIDE.md#readability).

### `path/file.go: baseline can shrink to 1 (was 2)`

This is a note, not a failure. You fixed a finding. Run
`lawgate readability-baseline` to record the lower count.

## CODEOWNERS

### `CODEOWNERS file is missing`

The platform's CODEOWNERS file does not exist. Create it:

```bash
lawgate codeowners --fallback > .github/CODEOWNERS
```

On GitLab the file is `CODEOWNERS` at the repository root.

### `.github/CODEOWNERS does not require @owner for: path/a.go, path/b_test.go`

These rule files have no entry that names the owner. This often happens
after you add a law test or a file it imports. Regenerate the entries with
the command above.

With `--base`, the message can name the CODEOWNERS of the base branch. A new
rule file must already be covered there. Keep a `* @owner` fallback, or
merge the CODEOWNERS entry first. See
[New rule files](USER_GUIDE.md#new-rule-files).

### `lawgate.json: owner requires a platform`

Add `"platform": "github"` or `"platform": "gitlab"`.

## Rule-change audit

### `cannot establish audit range from shallow history`

`rule-changes` needs the full history. In GitHub Actions, set
`fetch-depth: 0` on the checkout. In GitLab CI, set `GIT_DEPTH: '0'`.

### `<commit>: Rule-change-approves requires a Rule-change-approved-by audit trailer`

A commit has a `Rule-change-approves:` trailer without a
`Rule-change-approved-by:` trailer. Add the second one. Both are audit notes.
The approval itself is the owner's approval on the pull request or merge
request, which `rule-changes` checks when `require_approval` is on.

## The owner's approval

### `rule files changed (...): approval by @owner on <commit> is missing (...)`

A commit in the range changes rule files, and the owner has not approved
the pull request or merge request at its head. The reason in brackets says
what the platform answered:

- `@owner has not approved ...`: the owner approves it.
- `@owner approved <commit>, an older commit` (GitHub) or
  `@owner approved before the latest push` (GitLab): commits were pushed
  after the approval. The owner approves again.
- `@owner requested changes`, `@owner's review was dismissed` or
  `@owner revoked the approval`: the latest decision wins. The owner
  approves again.

On GitHub the approval runs the check again. On GitLab it does not: rerun
the `review-rules` job, or run a new pipeline for the merge request.

A pull request or merge request the owner authored passes without a
review (`authored by @owner: counts as approved`). While agents push and
open them with the owner's credentials, their changes pass the same way:
run agents under their own account to close that.

### `lawgate.json: require_approval requires an owner and a platform`

Set `owner` (a user, such as `@name`) and `platform` next to
`require_approval`, or remove `require_approval`.

### `... commit <commit> reached the default branch without a pull request`

On a push to the default branch, a rule-file commit has no merged pull
request or merge request. It was pushed directly, perhaps by the owner. The
approval is what the check reads, so every rule-file change goes through a
pull request or merge request. Revert the commit and open one.

### `... it is merged already: revert it, or land it again ...`

The pull request or merge request that merged the commit had no valid
approval. Revert the commit, or open a new pull request or merge request
the owner approves.

### `owner @org/team is a team or group: only a user can approve a rule change here`

A team or a GitLab group cannot approve a pull request or merge request.
Set `owner` in `lawgate.json` to the user who approves rule changes.

### `cannot check the owner's approval: ...` / `<VARIABLE> is not set`

With `require_approval` on at BASE (or `--require-approval`),
`rule-changes` reads the approval from the CI platform, and fails when it
cannot. With it off, `rule-changes` needs no token. Run it in the CI job. On GitHub, pass
`GITHUB_TOKEN: ${{ github.token }}` to the step and grant
`permissions: pull-requests: read`. On GitLab, add the masked CI/CD
variable `LAWGATE_API_TOKEN` with a `read_api` token; `CI_JOB_TOKEN` cannot
read approvals.

### `GET /...: 401 Unauthorized` or `403 Forbidden` or `404 Not Found`

The token cannot read the project's pull requests or merge requests. Check
the workflow's `permissions` on GitHub, or the scope (`read_api`) and the
project membership of `LAWGATE_API_TOKEN` on GitLab.

### `HEAD <commit> is not the head <commit> of pull request #N` / `... is not the latest version ...`

The range the job passes does not end at the pull request's head or the
merge request's latest push. Pass the head as HEAD. On GitLab, rerun the
pipeline of the latest push.

## Configuration and commands

### `json: unknown field "..."`

`lawgate.json` has a key that lawgate does not know. Check the spelling
against the [field list](USER_GUIDE.md#lawgatejson).

### `refusing to overwrite <file>`

`lawgate init` never replaces a file. Remove the file, or edit it by hand.

### `no Go modules found under <dir>`

`lawgate init` found no `go.mod`. Run it at the repository root, or pass
`--root DIR`.

### `command timed out after 5m0s`

A command of a module ran longer than its deadline. Raise `timeout_seconds`
for that module, up to 86400.

### `module "x": opaque Go machine command requires machine_build`

The machine command is a script or a Makefile target, so lawgate cannot read
its build settings. Add `machine_build` with `goos`, `goarch`, `tags` and
`cgo_enabled`.

### `unsupported Go flag -overlay`

lawgate accepts only the Go flags it can account for when it finds source
files and owners. Remove the flag, or move the command into a script and
list its inputs in `gate_files`.

### `resolves through the symbolic link ...: lawgate refuses symlinked inputs`

A rule file, a script or a command argument is reached through a symbolic
link. Replace the link with the real file.
