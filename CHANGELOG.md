# Changelog

All notable changes to lawgate are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions
follow [Semantic Versioning](https://semver.org/). Until 1.0.0, a minor
version may change the API; the entry says how to migrate.

## [Unreleased]

## [0.1.5] - 2026-11-27

### Added

- `check` says what it is doing while it runs: one line per step on stderr
  (the machine inventory, each module's machines, tests and laws, each law's
  bite as `bite N/M`, the repository checks and the proof lock), each with
  the time since the check began and how long the step took, and a closing
  line with the passed, failed and pending counts. stdout still carries only
  the report directory, so `report=$(lawgate check)` keeps working and a CI
  log is no longer silent for the length of the gate.

## [0.1.4] - 2026-11-25

### Changed

- `report`: the page is three tabs under the summary, each its own view:
  **Needs attention** (opens first when there is any), **Machines** and
  **Gate checks**, with counts on the tabs. The URL hash keeps the tab.
- `report`: machines are one row each (name, source, law count, verdict),
  folded; a row with a broken law starts open. The filter opens the rows
  that match.

## [0.1.3] - 2026-11-25

### Changed

- `report`: the gate checks have their own filter, a search box and the
  All, Broken, Pending and Holds chips. It filters single checks, hides the
  packages it leaves empty and opens the ones that match. Every check is in
  its package, with the package's broken, pending and holding counts on its
  line; a package with a broken check starts open.

## [0.1.2] - 2026-11-25

The HTML report listed every law and gate check at full length: Fleet's
ran to 93,000 px for a run where nothing broke. It now leads with what
needs attention and folds what holds (5,600 px for the same run).

### Changed

- `report`: the page starts with **Needs attention**, every broken or
  pending item grouped by reason, broken groups open and long pending
  groups folded.
- `report`: machines are sorted worst first, and the laws that hold fold
  under one line.
- `report`: gate checks take the full width after the machines; the prefix
  the Go tests share is shown once and cut from their names, and the checks
  that hold are grouped by package, folded, with a count. Filtering opens
  the folded parts that match.

## [0.1.1] - unreleased

The adopters are private repositories on the free plans of GitHub and
GitLab, where required code-owner review, branch protection, rulesets and
required checks are paid features. CI can now check the owner's approval
through the platform's API (owner decisions 2026-11-24).

### Added

- `lawgate.json` `require_approval` (default `false`; needs `owner` and `platform`): when true in `lawgate.json` at BASE, `lawgate rule-changes BASE HEAD` checks, in CI, that the owner approved the pull request or merge request behind every rule-file change. Read at BASE, so a change cannot switch it off for itself; the change that switches it on takes effect from the next change. Off, `rule-changes` prints only the audit and needs no token. `--require-approval` turns the check on for one run.
- The approval check: the owner is `lawgate.json`'s owner at BASE (a user, never a team or group). A pull request or merge request the owner authored counts as approved, and the verdict says so ("authored by @owner: counts as approved"). GitHub pull requests: the owner's latest deciding review is `APPROVED` on the head commit; a later request for changes or dismissal wins. GitLab merge requests: the owner is in the approvals and the owner's latest "approved this merge request" note is newer than the latest diff version and not followed by "unapproved". Pushes to the default branch: every rule-file commit comes from a merged pull request or merge request the owner approved or authored; a commit pushed without one is named. A merge queue run reads no approval. Outside CI or without `GITHUB_TOKEN` / `LAWGATE_API_TOKEN` it fails and names the missing variable.
- `approval`: the package behind the check (stdlib `net/http`, 30-second timeouts, `Link` and `X-Next-Page` pagination on the API host only, no token in errors).
- `rulechanges.AuditRange` returns HEAD's commit ID and the commits of the audited range that changed rule files, with their files.

### Changed

- `rule-changes-are-reviewed` (owner decision 2026-11-24): every rule file is listed for the owner in CODEOWNERS, and when `require_approval` is on CI's `rule-changes` checks the owner's approval on the pull request or merge request (one the owner authored counts as approved). Enforced also by the `approval` tests; subjects `approval.judgeReviews`, `approval.gitlabEvidence.verdict` and `approval.githubAPI.judgeCommit`.
- Approval is the owner's approval on the pull request or merge request, not the platform's code-owner review. CODEOWNERS stays the list of rule files (`codeowners` and `check-codeowners` are unchanged). README, the user guide, concepts, troubleshooting, the review guide and the website say so, and name the limit: while agents push and open pull or merge requests with the owner's credentials, their changes look authored by the owner and pass; a separate account for agents closes that. On GitLab Free "Pipelines must succeed" blocks a merge without the approval; on GitHub Free private repositories a missing approval is detected (red pull request check and the push check on the default branch), not prevented.
- `templates/github-actions.yml`: runs on `pull_request_review` (submitted, dismissed) and on pushes to the default branch, with `permissions: pull-requests: read`; `review-rules` passes `GITHUB_TOKEN` to `rule-changes`, which decides from `require_approval`; a review does not rerun `check-laws`. Pins lawgate v0.1.1.
- `templates/gitlab-ci.yml`: uses only GitLab Free. Merge request pipelines (no merged-results pipelines or pipeline execution policy) and push pipelines of the default branch, BASE from `CI_MERGE_REQUEST_DIFF_BASE_SHA` or `CI_COMMIT_BEFORE_SHA`; documents that `LAWGATE_API_TOKEN` and "Pipelines must succeed" are needed only with `require_approval` on, and the pipeline rerun after an approval. Pins lawgate v0.1.1.
- The CODEOWNERS fallback templates describe CODEOWNERS as the list of rule files the owner approves.
- lawgate's own `lawgate.json` sets `require_approval`, and its workflow runs on `pull_request_review`.
- The install lines (README, user guide, website, VS Code extension README) name v0.1.1.

## [0.1.0] - unreleased

The first release: the laws-and-proofs gate extracted from Pantacor Fleet
and the Pantavisor AI stack into one framework both repos share.

### Added

- The project website (<https://pantacor.github.io/lawgate/>) and the user docs in `docs/` (user guide, concepts, troubleshooting, developer guide), published with GitHub Pages.
- `rulechanges.RuleFiles` returns a `RuleFileList` with the rule files and the ownership notes (approved outside include directories, system headers).
- `machine`: a worker's control flow declared as data (states, steps,
  sites, edges with limits, catches), structural validation from the entry
  state, laws as Go functions over the graph, and two ordering helpers:
  `AlwaysBefore` (every path) and `SomePathBefore` (at least one path).
- `resolve`: binds every declared site to the source with `go/parser`, so a
  tag in a comment never counts; the tag pattern and the call vocabulary come
  from the adopting product.
- `lawsdoc`: the `LAWS.md` format (law headings with Source, Enforced by and
  Status) and its checker; enforcing tests are discovered under the build's
  constraints and must name their law.
- `report`: JUnit, markdown and HTML reports in which a crashed, missing or
  empty module is never shown green.
- `prove`: exhaustive checks of pure kernels over bounded inputs (layer 4).
- `gen` and `run`: generated state types with enforced edge limits, and a
  run-time trace that refuses discontinuous or undeclared moves (layer 3).
- `readability`: the owner's readability rules (nesting, inline nested
  literals, function length, long conditions, bool parameters) with a
  shrink-only baseline.
- `inventory` and `lawgate check`: one command that runs every module's
  machine laws, named law tests, `LAWS.md` checks, readability and
  CODEOWNERS coverage from `lawgate.json`, and fails closed on anything
  missing or empty.
- `rulechanges`: rule files found by what they declare, CODEOWNERS
  generation and coverage checks, and an audit of rule changes since the
  merge base.
- `lawgate init`: starter `lawgate.json`, baseline, CODEOWNERS and CI file
  (GitHub Actions or GitLab CI) for a new repo.
- `gen.Check` lists missing or stale files by comparing them with in-memory generation.
- `run` records declared catches and checks them during replay.
- `rulechanges`: `DeclaresMachines` reports whether a module's non-test Go declares a machine.
- `lawgate init --fallback` and `lawgate codeowners --fallback` start CODEOWNERS with `* OWNER`, so a new rule file is reviewed before the generated block lists it. lawgate's own CODEOWNERS uses it.
- lawgate runs its own gate in GitHub Actions on the merge candidate and the merge queue, with the verifier built from the base commit.

- `runner`: a shared machine runner (`runner.Main`) that always resolves each
  machine against its source before checking its laws, with a documented
  exit-code contract (0 clean, 1 a law or drift failed, 2 usage or I/O).
- `example/`: a small adopting module (one machine, laws with witnesses, a
  Go enforcing test and a `LAWS.md`) that lawgate's own gate checks end to
  end.
- CI templates check the merge candidate (GitHub `merge_group`, GitLab
  merged-results pipelines).
- The example generates worker state types and checks their freshness as a law.
- The example tests the per-pass retry limit through a load fault and the generated retry transition, in live runs and replay.
- The gate reruns agreed script and Node test enforcers with `LAWGATE_BITE` and requires failure, unless the law records a reason under `Bites: not yet.`.

- `timeout_seconds` per module in `lawgate.json` (default 300, 1 to 86400) bounds every command the gate runs for it, including test discovery.

- cgo ownership follows `#include`, `#cgo -I`, `-include` and `-imacros` from every file of a package, splits `#cgo` flags with Go's quoting rules, and reports an include that goes through a symbolic link, uses an unknown variable, is named by a macro, or leaves the repository outside the owner's `outside_includes` list in `lawgate.json` (approved directories and system headers are reported once as notes).
- A Node enforcer binds only to a real `test(...)`/`it(...)` call in a file the runner loads; `.jsx` and `.tsx` sources are followed through their imports.
- A recorded bite gap (`Bites: not yet.` or `Enforced by: not yet.`) is reported as pending with its reason in every report, never as a pass.
- A law enforced by a Go test names its subject in `LAWS.md` (`- Subject: <import path>.<Func>` or `<import path>.<Type>.<Method>`, several per law, one or more per line), or records `- Subject: not yet. <why>`, which stays pending; an agreed Go-test law with neither fails `laws-bite`. The subject may live in another module of the repository that the law's module builds against.
- Go bites are gate-chosen mutations of the subject (package `mutate`): each `if`, `for` and expression-less `switch` case condition and each named condition (a variable assigned a comparison or `&&`/`||`) is negated, each comparison in it flipped (`<`/`<=`, `>`/`>=`, `==`/`!=`), and each guarded return disabled; the law's enforcing tests must fail on every mutant, and a surviving mutant fails `laws-bite` naming its file, line and change. A subject with no guard condition fails too.
- Mutants are compiled through `go test -overlay` (in `GOFLAGS`), one run per mutant, at most 64 per subject (a truncated run says how many it checked); the repository is never written. The runs keep the configured `-C`, tags, behaviour flags and test-binary arguments and drop the flags that write files, so the configured profile still describes the normal run.
- A condition a function returns (`return strings.TrimSpace(job.Payload) != ""`) or declares with `var` is a guard too, mutated like an `if` condition; a condition is recognised by its form (a comparison, `&&`/`||` or `!`). The mutation cache key carries a rules version, so runs cached under older rules are checked again.
- Limit: a guard that only passes an error up unchanged (`if err != nil { return <zero values>, err }`, the error bare, wrapped by `fmt.Errorf` with `%w` or joined by `errors.Join`) is not mutated, since error propagation is not law behaviour; every verdict says "N error-propagation guards not mutated". Any other use of the error in a guard (`err != nil && …`, `errors.Is`, a branch that returns a value or changes state) is still mutated.
- A run that killed every mutant is cached under the user cache directory, keyed by the subject's fingerprint and the proof's (the enforcing tests and their package's test files); an unchanged law is not mutated again, and a survivor is never cached.
- Go code holds no test switch: the `bite` package offers no hook, only the `LAWGATE_BITE` name and the reserved `lawgate/` prefix that Node and script reruns use, and the `lawgate` command refuses to run while the variable is set.
- The example names `hasPayload` as the subject of `rejects-empty-payload`, a function that holds only that guard.
- A bitten script must exit 1 and print `lawgate: rejects <law>`; any other failure is an infrastructure error.
- A Node bite on a direct `node --test` command reruns only the tests whose name contains the enforcer's (`--test-name-pattern`); any other Node runner reruns its whole suite.
- When that pattern skips the enforcer (Node skips a `test()` nested in a parent whose name does not match), the rerun falls back to the whole test command. A rerun that still has no result for the named test is reported as "did not run", with or without the pattern.
- `LAWS.md` refuses a law whose name starts with the reserved `lawgate/` prefix, and reads no enforcer for it.
- A bite rerun that fails reports the named test's own output, never the raw `go test -json` stream.
- The summary lists items that wait on a decision under `### Pending`, apart from `### What broke`, and counts "1 failure" in the singular.
- Commands the gate runs refuse arguments that reach a repository file through a symbolic link, matching ownership.

- `readability` also checks function literals, nested slice and array
  literals, and nesting inside closures from the enclosing depth.
- `run` owns consecutive edge counts and refuses invalid moves in live runs and replay. Generated entry and transition methods now return errors, and generated limited methods take no count.
- `machine.CheckLaws` requires a witness that mutates a copy of the machine and produces a violation.
- `resolve` rejects silent failure sites declared as ordinary logs.
- `lawgate check` validates Go and Node laws with one enforcement plan, including duplicate sections and missing machine laws.
- `lawgate check` runs executable script enforcers from the repository root and reports each exit as a law verdict.
- Go law tests require execution evidence for their compiled import path and test name. Ambiguous bare names must be qualified.
- Machine evidence must cover the names declared in source or law documents, and an unregistered Go machine module fails the gate.
- JUnit machine evidence rejects skipped laws without an explicit pending reason.
- Gate subprocesses have a deadline and bounded output, and machine JUnit input is size-limited.
- Go test discovery accepts bodyless test declarations without panicking.
- Node law test names must be unique in TAP evidence, so a passing test cannot substitute for a skipped one with the same name.
- Rule files include everything an enforcing test or a rule declaration reaches: helpers, methods of reached types, selector chains into the repository's packages, and the relative imports of Node enforcers.
- Rule discovery skips `testdata`, `vendor` and `.`/`_` directories; a file that cannot be read or parsed is protected and reported instead of aborting `rule-files`, `check-codeowners` or the audit.
- CODEOWNERS patterns follow GitHub and GitLab: a rooted pattern ending in a wildcard (`/*`, `docs/*`) matches one level, and `**` crosses directories only as a whole segment. Team and group owners (`@org/team`, `@group/sub/team`) and `.`/`_` handles are accepted.
- The owner comes from `lawgate.json` (`--owner` overrides; with `--base`, from the base's `lawgate.json`); there is no built-in owner, and `init` requires `--owner`.
- Product package names left the classifier: products list theirs in `lawgate.json` `rule_packages`; `scripts/laws.sh` and `scripts/rule_changes.sh` are no longer built-in rule files (use `gate_files`).
- Script enforcer paths in `LAWS.md` resolve relative to that `LAWS.md`.
- Readability checks unnamed compound conditions in tagless switch cases.
- Every GitHub action in the CI template is pinned to a commit SHA with its release tag in a comment.
- CODEOWNERS fallback templates are embedded, including GitLab's mandatory Law gate section.
- The GitLab CI template documents the Node.js image requirement for Node modules.
- `run` counts each limited edge across the whole pass, including retry loops through other states, during recording and replay.
- `gen` reports Go identifier collisions among generated state types, outside target types, and entry functions.
- `machine.CheckLaws` requires witnesses to return the same copy they received and to change the declaration beyond report metadata.
- `resolve` binds a `Match` site to the innermost known call containing its matched text, including on continuation lines.
- Rule files are package-granular: every package the closure reaches is protected whole (all Go files including tests, `testdata/`, embedded files), with its module's `go.mod` and `go.sum`.
- Every law document configured in `lawgate.json` (`laws`, `shared_laws`) is protected and supplies enforcers wherever it lives, scoped to the modules that list it.
- Packages that build a `runner.Entry`, call `runner.Main` or declare a `resolve.Vocabulary` are rule code, and a machine command's package seeds the closure; imports are followed by package name when it differs from the directory's.
- Qualified test references (`import/path/TestName`, the form lawsdoc asks for) seed rule files the way the check resolves them, and `required_tests` no longer match every test with the same base name.
- `--owner` replaces only the owner: the platform still comes from `--platform` or `lawgate.json`. The ownership commands refuse flags they would ignore.
- `init --fallback` and `codeowners --fallback` write the platform's fallback block (GitLab's inside the `[Law gate]` section); `codeowners` takes `--platform`.
- Go module, machine and test discovery share source traversal. Machine discovery honors runner build constraints, reports unresolved declarations and unscanned sensitive paths, and requires an explicit build inventory for opaque Go runners.
- Machine evidence must contain each machine's named local laws and every shared law configured for its module.
- Law documents may use any configured repository-relative filename. Their sources must cite an existing repository path, a URL, or a dated owner decision.
- Script enforcers may use parent-relative paths that resolve inside the repository.
- Machine validation and state generation reject the reserved edge name `enter`.
- Machine witnesses mutate their copy in place, preserve the name and entry state, and demonstrate a failure only after the original machine passes.
- Generated edges all use `Run.Go`, which enforces declared limits. Replace calls to the removed `Run.GoLimited` with `Run.Go`.
- Markdown summaries render multiline failure details beneath the law name.
- The example generator supports `go run ./cmd/genstates -check` to reject stale worker state types without rewriting them.
- Rule files include every repository package an enforcing or rule-declaring package imports, transitively, blank imports included.
- Rule discovery reads `//go:embed` patterns with `go/build` (quoted names with spaces included); a directive it cannot evaluate protects the package's whole tree and is reported.
- A qualified test reference protects every build variant of that test; only execution requires the one active test.
- Rule discovery walks the repository with `gosource`'s one rule: Go source and `go.mod` are read whatever their names.
- Go source and manifests are scanned regardless of credential-like names; ownership and source discovery share path exclusions.
- Machine discovery resolves Go types and follows the direct runner's actual package or file operands and imports.
- Shared law documents validate metadata, duplicates and enforcers using the adopting module's evidence.
- Source citations resolve from the law document and require explicit owner-decision wording for dates.
- A script enforcer that cannot start is an infrastructure failure.
- Modules with generated Go source require a bounded `generate_check` command whose exit determines the freshness verdict.
- Ownership follows local Go replacements and workspace members, protects the workspace manifests, and reports unreadable local imports.
- Configured gate commands seed ownership for their Go packages and repository scripts, including the example freshness checker.
- The gate records per-target freshness from `gen.ReportCheck` and rejects empty evidence and uncovered generated files.
- Machine discovery resolves a constructor that sets a constant name on a zero-value machine before returning it.
- Machine discovery scans cgo sources without crashing: go/types cannot know the type of a cgo operand, so a conversion such as `int(C.f())` is no machine, and one to a machine type, such as `Machine(C.f())`, is an unresolved origin.
- A dated source citation still requires its repository path to exist and stay within the repository.
- A law's source must point outside its own document and cannot cite `go.mod` as its authority.
- Agreed law status dates use the same calendar validation as dated source citations.
- Ownership follows Go's vendor mode: in a module or workspace with `vendor/modules.txt`, every vendored package a reached package imports is protected whole and followed transitively (vendored local replacements included), with `vendor/modules.txt`; unreached vendored code is not parsed.
- Ownership reads a `_`-prefixed package an import reaches, protects the assembly, cgo and other non-Go build files of a reached package, and reports a `replace` outside the repository as "outside the repository: lawgate cannot protect it" (ignored when the build vendors).
- Ownership resolves a one-letter nested module as the inner module, and a module its nearest `go.work` omits builds on its own.
- Node files a configured command, script enforcer or gate file names are followed through their relative imports, like Node law tests.
- A script enforcer reached through a symbolic link is refused by ownership, as the check refuses to run it.
- Ownership reads only the vendor/ the build selects (the workspace's in workspace mode, else the module's), and waives an outside `replace` only when Go vendors by default (`go` 1.14 or later, `modules.txt` written for the same mode).
- Ownership refuses a symbolic link at or under a module's or workspace's `vendor/`, and a configured command argument or gate file reached through one.
- Node relative imports resolve like `require`: file, extensions, then a directory's `package.json` `main` and `index` file; the manifest is protected, and an import that loads no repository file is reported.
- Node files a configured command, script enforcer or gate file names are followed from `_` and `.` directories too.
- `check-codeowners` canonicalizes `--root`, so a root reached through a symbolic link works like `check`.
- `go.mod` and `go.work` raw-string (backtick) paths are read, and `lawgate.json` refuses a `rule_packages` entry with an invalid import path or a name that is not a Go identifier.
- `codeowners` and `check-codeowners` refuse a `--platform` other than `github` or `gitlab`.
- `init` without Git lists `vendor/`, `.github` and other hidden or `_` directories like Git would; only `.git` and `node_modules` are left out.
- `machine.Catch` has a `Limit` with the meaning of `Edge.Limit`, counted per source state by `run` in live runs and replay; `machine.Validate` requires it on a catch whose target holds the caught kind, and refuses a negative one.
- `run.Run.Go` refuses an empty edge name, and replay refuses a trace move that names neither an edge nor a catch.
- Rule discovery resolves a relative Node import as Node does for its module system: `require` tries the exact name, `.js`, `.json`, `.node`, then the directory's `package.json` `main` and `index` files, and loads the first match; an ES module `import` loads exactly the file it names; a TypeScript source protects every candidate a loader could pick. `.`, `..`, `./` and `../` specifiers resolve, and a `main` or import outside the repository or through a symbolic link is refused.
- A Node module's named test enforcer that no `test(...)` or `it(...)` call declares is reported, and every test file of the module is protected with its imports.
- A reached cgo package protects every repository header its preamble and C sources include, transitively, through quoted paths and `#cgo` `-I` directories; an include lawgate cannot resolve protects the package's whole directory tree.
- Every rule file reached through a symbolic link is refused, the readability baseline, law documents and CODEOWNERS included; a symlinked `vendor/` is reported once, and a command's selected `GOWORK` workspace has its `vendor/` checked too.
- A failing Node bite control or mutation reports only the named test's own TAP block (`not ok` line and YAML diagnostics), not the whole TAP stream.
- Rule discovery checks cgo include and forced-header paths for symbolic links as written, protects every repository header the compiler's search order could supply, and reports `#cgo pkg-config:` directives while protecting the package tree.
- `outside_includes` refuses `/` and any directory that contains or is the repository root.
- `lawgate lock` pins agreed Go test functions, their reachable helpers, and protected subjects in the owner-reviewed `LAWS.lock`.
- `lawgate check --base` replays approved Go proofs against candidate code and checks whether the base lock authorizes removing an agreed law. The supplied CI templates pass the base commit.
- `lawgate review [--base REF] [--json]`: a branch report (Markdown or JSON) that ties every changed hunk to the machine states, sites, law subjects, enforcing tests and laws it touches, with their verdicts, the commits behind it, the rule changes (classified by `rulechanges`) and their audit trailers, and the proof or subject changes of `LAWS.lock`; `--pr` asks `gh` or `glab` for the pull or merge request of each commit.
- `lawgate review -lsp`: a language server for editors (code lenses, hover, go to definition, diagnostics and the `lawgate.review`, `lawgate.reviewMarkdown`, `lawgate.why`, `lawgate.reindex`, `lawgate.summary` and `lawgate.index` commands) that re-indexes a second after a `.go` file or a `LAWS.md` is saved. See `docs/REVIEW.md`.
- `runner`: `-index FILE` writes the versioned machine index (`review/index`) that `lawgate review` reads: states, sites resolved to their source line and declaration, law verdicts and drift.
- VS Code extension `editors/vscode` ("Lawgate"): the branch review view, notes on changed lines, the report panel, "why did this line change?" and the laws' verdict in the status bar, served by `lawgate review -lsp`; CI builds `lawgate.vsix` as an artifact.
- `laws`: the worker laws Fleet and the Pantavisor AI stack share, as constructors that return a `machine.Law` with its witness, under the names their `LAWS.md` files use: `NothingSilent`, `EveryFailureSaysWhy`, `ReachesOneOf`, `NoStuck`, `WaitsEnd`, `DoneMeansStored` and `OneShotFailsOnFault`; product facts (terminal states, site and machine labels, site kinds) are parameters.
- `machine`: a step is identified by its state and its id. A step id may repeat across states (`Validate` refuses it only twice in one state); `AlwaysBefore` and `SomePathBefore` cover every occurrence of an id, and `StepRef`, `AlwaysBeforeRef`, `SomePathBeforeRef`, `StepIn` and `StatesWithStep` name one occurrence by its state.
- `render`: the machine outline (`Text`, plain or ANSI), the one-state agent brief (`Brief`) and the site count (`Summary`), merged from Fleet's and the AI stack's renderers.
- `runner`: `-list` (machines with state and law counts), `-machine NAME` (narrows any run to one machine), `-json` (one machine with its drift, verdicts and source lines, a `MachineView`), `-brief STATE` and the default outline with `-color`, next to `-laws`, `-junit` and `-index`.
- `runner -index`: two sites that share a tag or match (one call reached from two states) each point at their own declaration literal (`gocode.Literals.FindFree`).
- `machine.Catch` with an empty `To` carries on: the pass stays in its state and the note says where the error is logged. `Validate` accepts it (and refuses a limit on it), the graph helpers see no edge, `run.Run.Catch(from, kind, "")` records no move, and `render`, the runner index (`Site.Caught`), `lawgate review` and the VS Code notes show it as "carries on".
