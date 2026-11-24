# lawgate

[![Law gate](https://github.com/pantacor/lawgate/actions/workflows/lawgate.yml/badge.svg)](https://github.com/pantacor/lawgate/actions/workflows/lawgate.yml) [![Website](https://github.com/pantacor/lawgate/actions/workflows/pages.yml/badge.svg)](https://pantacor.github.io/lawgate/)

Lawgate runs repository rules against Go and Node modules and publishes the
results as Markdown, HTML, and JUnit. It uses Go 1.25 and the standard library.

The website is at <https://pantacor.github.io/lawgate/>. It renders the guides
in [`docs/`](docs/): the [User Guide](docs/USER_GUIDE.md),
[Concepts](docs/CONCEPTS.md), [Troubleshooting](docs/TROUBLESHOOTING.md) and
the [Developer Guide](docs/DEVELOPER_GUIDE.md). This README is the quick
reference.

## Adopt lawgate in a repo in 5 steps

1. Install a pinned release with
   `go install github.com/pantacor/lawgate/cmd/lawgate@v0.1.0`. Use the same
   pinned release in CI. From a local checkout, run `go run ./cmd/lawgate`.
2. Run `lawgate init --platform github --owner @name` (or use `gitlab`).
   Review the discovered Go modules, correct their law document paths, and
   add any Node modules.
3. Write the module law documents using the format below. For Go machines,
   declare laws in `machine.Machine`, bind sites with `resolve`, and call
   `runner.Main` from `./cmd/machine` (see `example/` and the machine-runner
   contract below). Name each test's law in its doc comment or a string in its
   body, and name the function each Go test law protects in a `Subject:`
   line. Run `lawgate lock` and commit `LAWS.lock`.
4. Run `lawgate check`. For existing readability findings you intend to
   ratchet down, run `lawgate readability-baseline` and review the counts.
   Tell contributors in `AGENTS.md` to read the relevant laws and run the
   gate before committing.
5. Put the CI template in the protected workflow or pipeline policy and
   require its jobs and code-owner approval through branch protection. The
   GitHub template checks the PR merge candidate, supports `merge_group`, and
   pins every action to a commit SHA with its release tag in a comment. The
   GitLab template requires merged-results pipelines. For Node modules, use a
   GitLab job image with Go and Node.js. Review the output of
   `lawgate codeowners --owner @name` when rule files change. Approval is
   recorded by the platform review. The Git trailer is an audit note.

## Commands

| Command | What it does |
| --- | --- |
| `init --owner @name [--root DIR] [--platform github\|gitlab] [--fallback]` | Discovers one module for each project `go.mod` and guesses `LAWS.md` beside it. Writes `lawgate.json`, an empty `.lawgate-readability`, a CODEOWNERS block, and the selected pinned CI template. Refuses existing destinations. `--owner` is required; the platform defaults to `github`. `--fallback` starts CODEOWNERS with `* @name` (off by default). |
| `check [--root DIR] [--out DIR] [--base REF]` | Checks inventory, machines, test execution, and law documentation, mutates each law's subject, compares proofs and subjects with `LAWS.lock`, then runs configured readability and CODEOWNERS checks. With `--base`, also replays the base ref's proofs against the candidate code. Prints a fresh run directory under `laws-report` by default. The directory holds `laws-summary.md`, `laws-report.html`, and numbered JUnit files for each module and any repository-wide checks. A missing module, crashed runner, or empty report fails the gate. |
| `lock [--root DIR]` | Writes `LAWS.lock`: for each law, the fingerprint of its proof (the enforcing tests and the test functions they call) and of its subject. `check` fails when either differs from the lock. The lock is a rule file. |
| `readability-baseline [--root DIR]` | Rewrites the configured baseline from current findings as one `count path` line per offending file. |
| `rule-files [--root DIR]` | Lists rule files: configuration, the baseline, law documents, gate files, CI files, and the source and tests that declare or enforce rules with everything they reach. A file that cannot be parsed is listed and reported (exit 1). |
| `codeowners [--root DIR] [--owner @name] [--platform github\|gitlab] [--fallback]` | Prints exact CODEOWNERS entries for the current rule files, after the platform's fallback block with `--fallback`. |
| `check-codeowners [--root DIR] [--owner @name] [--platform github\|gitlab] [--base REF]` | Requires owner coverage in the active CODEOWNERS and, with `--base`, also in the base ref's active CODEOWNERS. |
| `rule-changes [--root DIR] BASE HEAD` | Audits rule changes from the merge base through HEAD, including renames and merges, and reports approval trailers. |

`--root` defaults to the current directory, and relative `--out` paths are
relative to that root. The ownership commands take the owner and platform
from `lawgate.json`; `--owner` and `--platform` override them. With
`--base`, the owner comes from the base ref's `lawgate.json`, so a change
cannot name its own reviewer. lawgate has no built-in owner.

## Rule files and CODEOWNERS

A rule file needs the owner's review. This includes every configured law
document, `lawgate.json`, the baseline, `gate_files`, CI files, and
CODEOWNERS. Repository scripts and Go packages invoked by configured gate
commands also need coverage, including `generate_check`. Lawgate follows
relative imports from Node tests, commands, script enforcers, and gate
files. CommonJS `require` tries the exact path, then `.js`, `.json`, and
`.node`, then the directory's `package.json` `main` and index files. An
ES module `import` requires the exact file. For TypeScript, lawgate
protects every resolution candidate. If a named Node enforcer matches no
`test()` or `it()` call in the source scan, lawgate protects every file
that `node --test` discovers by default and reports the unresolved
enforcer. List inputs behind opaque command wrappers in `gate_files`.

For Go, ownership follows local imports from the selected module or
workspace, including workspace members, local replacements, and explicitly
imported `_`-prefixed directories. Each reached package is protected in full,
including its Go source and tests, non-Go build inputs such as `.s`, `.c`,
`.h`, and `.syso`, `testdata/`, embedded files, and the controlling
module and workspace manifests and checksums. In vendor mode, ownership
follows imports through `vendor/` and protects every reached vendored package
and `vendor/modules.txt`. An unreadable local import or a replace target
outside the repository fails ownership discovery. In vendor mode, an
out-of-repository replace can use the in-repository vendored copy.

For cgo, lawgate follows the headers a reached package's compiler reads:
the quoted and bracketed `#include` and `#import` directives of its cgo
preambles and C files, and the headers they include in turn, and every
header a `#cgo` `-include` or `-imacros` option forces in. `#cgo`
`CFLAGS`, `CPPFLAGS`, `CXXFLAGS` and `FFLAGS` are split with Go's own
quoting rules, and their `-I`, `-iquote`, `-isystem` and `-idirafter`
directories are searched, with `${SRCDIR}` expanded, from every Go file
of the package. GCC takes the first match in a fixed order (for a quoted
include, the including file's directory and `-iquote`, then `-I`,
`-isystem`, the system directories and `-idirafter`), not the order the
flags are written in, so every repository header that any directory in
that search could supply is protected, whichever one the compiler reads.
A quoted include found nowhere protects the whole package tree. Each path
is walked as written, one element at a time, the way the operating system
opens it: in `link/../x.h` the `..` leaves the link's target, so the link
is met and refused, never cleaned away first. These are reported as
problems, with the package tree protected, because no CODEOWNERS entry
could cover what the compiler reads:

- a header or include directory outside the repository;
- a path through a symbolic link;
- a variable other than `${SRCDIR}`;
- an include named through a macro (`#include HEADER`);
- a `#cgo` flag list with an unclosed quote;
- a `#cgo pkg-config:` line: its flags come from outside the repository
  when the package is built, so they cannot be reviewed;
- a bracketed include found in no repository directory, when an outside
  include directory the owner has not approved could hold it.

The owner can approve outside directories in `lawgate.json`, which is a
rule file itself:

```json
{ "outside_includes": ["/usr/include/libfoo", "/opt/sdk/include"] }
```

Each entry is a clean absolute path, never `/`, and never the repository
root or a directory that holds it (that would approve every repository
file and neighbouring checkouts). An include directory or header inside
one is accepted and noted once for each directory. Bracketed headers the
compiler finds in its own system directories (`<stdio.h>`) are noted once
for each package, not reported.

Lawgate resolves a symlinked repository root before checking its files.
Script enforcers resolve relative to their law document. A symlink in a
script enforcer's path inside the repository is an error. Both `check`
and `readability-baseline` refuse a symlink in the readability baseline's
path.

`check-codeowners --base` checks the candidate's rule files against the
base's CODEOWNERS, so a new rule file must already be covered there. Two
ways to bootstrap one:

- Keep a fallback (`lawgate init --fallback`, or `* @owner` first in
  CODEOWNERS). The new file is covered at once; regenerate the exact block
  with `lawgate codeowners --fallback` in the same change.
- Without a fallback, land the CODEOWNERS entry (or a directory pattern
  such as `/workers/ @owner`) first, then the rule file.

The first change that adds lawgate has no base `lawgate.json` or
CODEOWNERS: the owner reviews and merges it before making the CI jobs
required.

## `lawgate.json`

Unknown keys are errors. Paths are slash-separated and relative to the
repository root. Command arguments are arrays of strings run from the module.

| Field | Type | Meaning |
| --- | --- | --- |
| `modules` | nonempty array of module objects | Required inventory of expected checks. |
| `product` | string | Optional product name shown in the HTML report. |
| `lock` | string | Optional path of the proof lock. Defaults to `LAWS.lock`. |
| `gate_files` | array of strings | Optional extra paths that require rule-file protection. |
| `owner` | string | Optional required CODEOWNER such as `@name` or `@org/team`. Enables coverage checking and requires `platform`. |
| `outside_includes` | array of strings | Optional clean absolute directories outside the repository that the owner approves for cgo include directories and headers. Not `/`, the repository root or a directory holding it. Each is noted once instead of reported. |
| `rule_packages` | array of `{"path", "names"}` | Optional Go packages whose declarations are rules, such as a product's own state machine package. Using one of `names` (every name when empty) makes a file a rule file; a `Machine` name also marks the module as declaring machines. |
| `platform` | `"github"` or `"gitlab"` | Optional CODEOWNERS platform. |
| `readability` | object | Optional Go readability ratchet configuration. |
| `readability.baseline` | string | Required with `readability`. Path to the protected baseline. An empty file allows zero findings. |
| `readability.include` | array of strings | Optional repository-relative globs. Defaults to all Go files in the listed Go modules. |
| `readability.exclude` | array of strings | Optional globs that take precedence over includes. |

| Module field | Type | Meaning |
| --- | --- | --- |
| `path` | string | Required module directory, unique in the inventory. Use `"."` for the root. |
| `language` | `"go"` or `"node"` | Required test runner type. |
| `laws` | string | Required path to this module's `LAWS.md`. |
| `machines` | boolean | Required. Set to true when a machine report is expected. |
| `machine_command` | array of strings | Allowed with `machines: true`. Defaults to `["go","run","./cmd/machine"]` for Go and is required for Node machine modules. Lawgate appends `-laws -junit FILE`. |
| `machine_build` | object | Required only when a Go module's `machine_command` is not a plain `go run`/`go test` invocation (a script, a Makefile target): `{"goos","goarch","tags","cgo_enabled"}` tells lawgate which build the runner uses, so machine discovery sees the same files. Refused otherwise. |
| `test_command` | array of strings | Optional override. The Go default is `["go","test","-json","-count=1","./..."]` and the Node default is `["node","--test","--test-reporter=tap"]`. Overrides must emit the same evidence format. |
| `required_tests` | array of strings | Optional extra tests that must run to completion. For Go, use a test name or `import/path/TestName`. |
| `shared_laws` | array of strings | Optional repository-relative paths to shared law documents. The same field and enforcer checks apply to every document. Every expected machine in the adopting module must report each shared machine law. Use `` machine `*` `` to name all adopting machines without naming a product-specific machine. |
| `generate_check` | array of strings | Command and arguments to verify generated Go files, run from the module directory. Required when the module contains any Go file with a standard generation header. |
| `timeout_seconds` | integer | Per-command deadline for this module, from 1 to 86,400 seconds. Defaults to 300. |

Direct `go run`, `go test`, and `go build` commands use the same argument
parser for execution, source discovery, and ownership. Use `-C DIR` first
after the subcommand to select a working directory. The parser accepts both
`-flag value` and `-flag=value` for value-taking flags, including `-tags`,
`-mod` (`mod`, `readonly`, or `vendor`), and `-modfile`. Ownership
covers the alternate `.mod` file and any matching `.sum` file. An `env`
prefix can set `GOOS`, `GOARCH`, `CGO_ENABLED`, and `GOWORK`. Use
`GOWORK=off` to disable workspace mode or an absolute path to a workspace
file inside the repository. Ownership covers that workspace and its
checksum when present.

Direct commands accept the standard test-selection, test-output, and
profiling flags and the build switches `-race`, `-asan`, `-msan`,
`-trimpath`, and `-cover`. They reject `-overlay`, `-exec`, `-toolexec`,
`-pgo`, `-pkgdir`, other unclassified Go flags, and a `GOFLAGS` prefix.
Lawgate runs direct Go commands with ambient `GOFLAGS`, `GOWORK`, and
Go's per-user configuration cleared. The automatic workspace search stops
at the repository root. Put custom test-binary flags after
`-args`. For an opaque machine runner, specify its build with
`machine_build` and list wrapper inputs in `gate_files`.

Readability globs support `*`, `?`, character classes, and `**` as a whole
path segment for zero or more directories. The readability check scans Go
tests and skips hidden directories, `vendor/`, `testdata/`, and unlisted
nested Go modules.
New or growing finding counts fail. Lower counts produce summary notes.

Source discovery reads Go files and `go.mod` regardless of names such as
`secrets.go` or `secrets/go.mod`. The credential-name filter applies to
non-source files, including `.env*` files and credential-like names.
Go source and module manifests reached through a symbolic link are refused.

## `LAWS.md`

A law uses a level-two or level-three heading with a kebab-case name. Other
headings group sections. A name under the `lawgate/` prefix is refused: the
prefix is reserved for lawgate's own control selectors. Write the rule and its owner-approved source.

```markdown
### law-name

One sentence describing the rule.

- Source: <repository path, URL, or owner decision YYYY-MM-DD>
- Enforced by: machine `worker` | test `TestName` or `import/path/TestName` | script `path` | not yet. <why> | proposed
- Status: agreed <YYYY-MM-DD> | proposed
- Subject: <import path>.<Func> | not yet. <why> (an agreed law enforced by a Go test)
- Bites: not yet. <why> (optional for a Node test or script enforcer)
```

Identify the source with a repository path, URL, or explicit
`owner decision YYYY-MM-DD`. A dated citation may use the form
`path/to/plan.md (agreed YYYY-MM-DD)`. Dated and undated source paths
resolve from the directory of the document and must name an existing file
inside the repository. Script paths follow the same rules. Script enforcers execute from the repository root.
Choose the applicable enforcers. Go checks machine law names, source and
status fields, script paths, and that named Go tests compile for the host
build, name the law, and ran to completion. Node checks the three fields and
looks for each named test in the TAP results. `Enforced by: not yet. <why>`
reports an agreed law as pending with its reason. `Law.Pending` is the reason
a verdict waits on a decision instead of failing: a machine-law violation
under owner review, a recorded enforcement, subject or bite gap, or an enforcing test
that did not run. Reports list these under their own "Pending" heading.
Shared documents use the adopting module's machine and test evidence. A law
should have a single section across that module's primary and shared documents.

## Making a law bite

A law bites when its enforcer fails once the protected behaviour is gone.
Production code carries no test switch: lawgate breaks the code itself, in a
copy, and pins the proof so that a change cannot rewrite the test that
judges it.

### Subject and mutation (Go tests)

An agreed law enforced by a Go test names the function it protects:

```markdown
- Subject: example.com/shop/wallet.Withdraw
```

Write `<import path>.<Func>` or `<import path>.<Type>.<Method>`. The
function must be in the law's module. For each subject, lawgate generates
mutants and runs the law's enforcing tests against each one:

- it negates each `if`, `for` and `switch` case condition;
- it flips each comparison (`<` and `<=`, `>` and `>=`, `==` and `!=`);
- it replaces the condition that guards a `return` of an error by `false`.

Each mutant is a temporary file applied with `go test -overlay`: the
repository is never edited. The enforcing tests must fail for every mutant.
A mutant that survives is a failed `laws-bite` verdict that names the
subject, the line and the mutation: either the test does not exercise that
condition, or the subject is not the function the law protects. lawgate
chooses the mutations, so a guard cannot be moved somewhere the test does
not look.

lawgate runs a bounded number of mutants per subject and reports when it
truncated the list. Results are cached by the fingerprints of the subject
and the proof, so an unchanged law is not mutated again.

Keep a subject small: a function that holds the law's guard and little
else. A subject with unrelated conditions produces mutants the law's test
has no reason to catch.

```go
// hasPayload is the subject of rejects-empty-payload.
func hasPayload(job Job) bool {
	return strings.TrimSpace(job.Payload) != ""
}
```

When a law has no subject yet, record the gap. It is reported as pending
with its reason, never as a pass:

```markdown
- Subject: not yet. The check lives in a generated file.
```

### The lock (`LAWS.lock`)

`lawgate lock` writes `LAWS.lock` at the repository root (or the path in the
`lock` field of `lawgate.json`). For each law it records two SHA-256
fingerprints:

- the proof: the source of each enforcing test function and of every
  function in the module's test files it calls;
- the subject: the source of the subject function.

Both are normalised by the Go printer, so formatting does not change them.
`lawgate check` recomputes them and fails when one differs from the lock:

```
proof of rejects-empty-payload changed: update LAWS.lock, which needs the owner's review
```

`LAWS.lock` is a rule file, so the owner reviews every change to it through
CODEOWNERS. Editing a law's test or its subject therefore needs the owner,
even when the gate would otherwise pass.

### Base replay (CI)

`lawgate check --base <sha>` replays the approved proofs against the
change. For every law agreed at the base commit, lawgate takes the base
version of the enforcing test files and the base `LAWS.lock`, overlays them
on the candidate tree and runs the named tests against the candidate's
production code. A failure names the law:

```
the approved proof of rejects-empty-payload fails against this change
```

A law that was agreed at the base and is removed or renamed by the change
also fails, until the owner updates the lock. The CI templates pass
`--base` with the merge base. Run it locally with
`lawgate check --base origin/main`.

Together the three checks close the loop: mutation shows the proof is not
vacuous, the lock shows who approved the proof, and base replay shows the
change still satisfies the proof that was approved.

### Node tests and scripts

Node and script enforcers have no subject and no mutation yet. They keep
the earlier model: the gate reruns the enforcer with `LAWGATE_BITE` set to
the law's name, and the enforcer must fail. This is a known gap: the code
that reads the variable is a test switch the author placed.

The enforcer must pass normally and pass a control rerun with an unrelated
selector under the reserved `lawgate/` prefix. Match the exact law name so
the control passes. For a Node enforcer, the named TAP test must fail. When
the test command starts `node --test` directly (the default), lawgate adds
`--test-name-pattern` with the enforcer's name, so each rerun runs only the
tests whose name contains it. Any other runner, such as `npm test` or a
script, reruns its whole suite twice per law. Node matches the pattern
against each test's own name, so it skips a `test()` nested in a parent
`test()` whose name does not match (a `describe` suite always runs). When a
narrowed rerun has no result for the enforcer, lawgate reruns the whole
test command once; if the test is still absent, the bite fails with "did
not run". A script exits 0 on a passing check. When its assertion rejects
the mutation, it prints the exact line `lawgate: rejects <law>` to stdout
or stderr and exits 1. Other nonzero exits, including exit 1 without that
line, are infrastructure failures. Keep the mutation local to the rerun.

The lawgate CLI reserves `LAWGATE_BITE` for these reruns and refuses to
start when the variable is set. Go code never reads it.

If such a law's breaking case is still pending, add
`- Bites: not yet. <why>` to report it as pending with its reason.

### Machine laws

A machine law demonstrates a bite with its declared witness. It needs no
subject.

Package `laws` offers the worker laws products share (`nothing-silent`,
`every-failure-says-why`, `done-means-stored`, `one-shot-fails-on-fault`,
and the parameterised `ReachesOneOf`, `NoStuck` and `WaitsEnd`), each with
a witness; see the [User Guide](docs/USER_GUIDE.md#declare-a-machine-and-its-runner).
A step is identified by its state and its id: a step id may repeat across
states, and `AlwaysBefore` and `SomePathBefore` mean every occurrence of an
id unless a `machine.StepRef` names the state
([Concepts](docs/CONCEPTS.md#machine)).

A catch with an empty `To` carries on: the pass stays in its state and goes
on with the next step, and its `Note` says where the error is logged, as in
`machine.Catch{On: machine.SiteWarn, Note: "the action's log"}`. It is no
edge of the graph, `run` records no move for it (`Run.Catch(from, kind, "")`
only checks that the pass can take it), and the outline, the brief and the
machine index show it as "carries on". `Validate` still refuses a catch to a
named state that is not declared, and refuses a limit on a carry-on catch.
`nothing-silent` still reports a silent site whose catch carries on, and
`one-shot-fails-on-fault` reports a checked kind whose catch carries on.

## Generated Go source

Lawgate detects the standard `// Code generated ... DO NOT EDIT.` header in
the repository's Go source tree, including tests and files excluded by build
tags. Give the owning module a `generate_check` command in `lawgate.json`.
Use `gen.ReportCheck(os.Stdout, targets...)` to regenerate each target in memory
and compare its content with the file on disk. The command runs from the
module directory with the gate's normal timeout and output limits. Custom
checkers should emit one stdout line per target with tab-separated
fields `lawgate-freshness`, `fresh` / `stale` / `missing`, and the
module-relative path. They should exit nonzero on failure.

Lawgate records a `generated-is-fresh` verdict for every reported target.
It fails for empty evidence, a generated Go file without a matching target,
a stale or missing target, or a failed command. The example uses
`["go", "run", "./cmd/genstates", "-check"]`.

## Machine runner contract

Go modules with `machines: true` should call `runner.Main(entries)` from
`./cmd/machine` (package `github.com/pantacor/lawgate/runner`). For each
`runner.Entry`, the shared runner always reads the declared Go source file,
binds it with `resolve.Resolve`, checks every law with `machine.CheckLaws`,
and writes `report.WriteJUnit` output — so a module cannot skip source-drift
detection.

Go machine discovery follows a direct runner's package or explicit file
operands, build flags and in-module imports. It uses Go types to recognize
machine declarations, including aliases, copies and values returned by
constructors, embedded machines, and a local machine named with a constant
in the same block before it is returned or passed on.
Machine names must be statically resolvable. For opaque runners
lawgate scans the module under its configured `machine_build`, and modules
without machine runners are scanned under the default build.

`lawgate check` invokes the machine command from the module directory with
`-laws -junit FILE` appended and enforces this exit-code contract for both
`runner.Main` and custom machine runners:

- **`0`**: every machine resolved cleanly and every law holds (or only
  `Pending` laws reported violations), and the JUnit report was written.
- **`1`**: the JUnit report was written and refuses the change (at least one
  broken law or a resolution/infrastructure failure recorded in the report).
- **Any other non-zero exit code (such as `2`)**: usage, flag, or I/O error.
  `lawgate check` records an additional `machine command` infrastructure
  failure even if a report file exists.

Running `cmd/machine -laws` without `-junit` prints a Markdown summary to
standard output and returns the same `0` / `1` / `2` exit status.

`runner.Main` also shows the machines (package `render`): with no action
flag it prints each machine's outline (`-color` colours it), `-list` lists
the machines with their state and law counts, `-json` writes one machine,
its drift, verdicts and source lines as a `runner.MachineView`, and
`-brief STATE` writes the brief an agent needs to change one state.
`-machine NAME` narrows any run, `-laws` and `-index` included, to one
machine. The views exit `0` when shown, `1` when shown over drift, and `2`
on usage or I/O errors.

## Review in your editor

`lawgate review` shows, for a branch, what governs each change: the machine
state and site a line belongs to, the laws that cover it and whether they
hold, the subject a law protects and the tests that prove it, and the commit
behind the line. It reads the same `lawgate.json`, `LAWS.md` files and
machine commands as `lawgate check`, in any adopting repository.

```bash
lawgate review --base main          # Markdown report (default base: the remote's default branch)
lawgate review --base main --json   # the same report as JSON
lawgate review -lsp                 # language server on stdio
```

The report groups every changed hunk by the states, sites, subjects, tests
and laws it touches, lists the commits that touch a rule file with their
approval trailers, and calls out a changed `LAWS.lock` (a proof or subject
changed: the owner reviews it). The language server gives code lenses
(`⚙ state`, `◆ site`, `🛡 protected by law`, `🧪 proves law`, each law's
verdict on its heading), hover, go to definition, diagnostics and the
`lawgate.review`, `lawgate.reviewMarkdown`, `lawgate.why`, `lawgate.reindex`,
`lawgate.summary` and `lawgate.index` commands; it re-indexes a second after
a `.go` file or a `LAWS.md` is saved. The VS Code extension is in
[`editors/vscode`](editors/vscode/); Neovim and Helix take a few lines of
configuration.

Machines are read through each module's machine command, run with
`-index FILE` instead of `-laws -junit FILE`: `runner.Main` writes the
versioned index (package `review/index`); a custom machine command must
accept the flag too. See [docs/REVIEW.md](docs/REVIEW.md).

## Example adopting module

The [`example/`](example/) directory is a minimal Go adopting module showing
the full end-to-end wiring:

- `example/worker.go` — imperative Go code with tagged failure and write
  sites, and `hasPayload`, the subject of `rejects-empty-payload`.
- `example/machine.go` — `worker` machine declaration, `resolve.Vocabulary`,
  and two machine laws (`no-silent-errors`, `validate-before-store`) with
  witnesses.
- `example/cmd/machine/main.go` — calls `runner.Main([]runner.Entry{worker.Entry()})`.
- `example/worker_test.go` — Go unit test enforcing `rejects-empty-payload`,
  freshness check calling `gen.Check`, and runner and drift checks.
- `example/states_gen.go` — generated state types. Run `go generate ./...`
  from the example module to refresh them.
- `example/retry_test.go` — uses the generated retry transition and the
  declared load-fault catch to show the per-pass limit in live runs and replay.
- `example/LAWS.md` — law document naming the machine and test enforcers and
  each test law's subject.

In `lawgate.json`, a Go machine module is listed as:

```json
{
  "path": "example",
  "language": "go",
  "laws": "example/LAWS.md",
  "machines": true
}
```

`example/go.mod` uses `replace github.com/pantacor/lawgate => ..` so it runs
offline inside this repository; external adopting repositories should require
a pinned release version instead.

## What a law over a machine does not see

A `machine.Law` checks the declared `machine.Machine` graph — states, steps,
edges, catches, and declared sites — not arbitrary Go control flow. Package
`resolve` closes layer-2 drift for every tagged log or failure site by
verifying that every tag in the Go source is claimed by a declared site and
that each site's kind and label calls match the code.

An untagged Go branch (for example, an `if` check or early `return` with no
tagged call matching the product's `resolve.Vocabulary`) is invisible to both
`machine` and `resolve`. Close that gap with:

- runtime transition guards (`package run` and `package gen`) so undeclared
  moves, exceeded edge limits and exceeded catch limits fail immediately at
  run time (a catch back into its own state must declare a limit),
- focused Go unit tests named under `Enforced by: test ...` in `LAWS.md`, or
- exhaustive domain checks with `package prove`.
