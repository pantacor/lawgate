# Changelog

All notable changes to lawgate are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions
follow [Semantic Versioning](https://semver.org/). Until 1.0.0, a minor
version may change the API; the entry says how to migrate.

## [Unreleased]

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
- The gate reruns agreed test and script enforcers with `LAWGATE_BITE` and requires failure, unless the law records a reason under `Bites: not yet.`.

- `timeout_seconds` per module in `lawgate.json` (default 300, 1 to 86400) bounds every command the gate runs for it, including test discovery.

- cgo ownership follows `#include`, `#cgo -I`, `-include` and `-imacros` from every file of a package, splits `#cgo` flags with Go's quoting rules, and reports an include that goes through a symbolic link, uses an unknown variable, is named by a macro, or leaves the repository outside the owner's `outside_includes` list in `lawgate.json` (approved directories and system headers are reported once as notes).
- A Node enforcer binds only to a real `test(...)`/`it(...)` call in a file the runner loads; `.jsx` and `.tsx` sources are followed through their imports.
- A recorded bite gap (`Bites: not yet.` or `Enforced by: not yet.`) is reported as pending with its reason in every report, never as a pass.
- Bite reruns are scoped: lawgate's own hooks use the reserved `lawgate/` prefix and are recognised by the module path its own `go.mod` declares, a hook counts only when the enforcing test's package reaches it, and a control rerun naming an unrelated law must pass first. When that control fails, a rerun with an empty selector tells a broken tagged build from an enforcer that rejects every selection.
- Go bite reruns keep `-C`, behaviour flags (`-short`, `-timeout`, `-race`) and test-binary arguments, with one merged `-tags`. They drop the flags that write files (`-coverprofile` and the other profiles, `-trace`, `-outputdir`), so a rerun never replaces what the configured command wrote.
- `bite.Off` is active only in builds with the `lawgate_bite` tag, which the gate's bite reruns pass; any other build ignores `LAWGATE_BITE`, and the `lawgate` command refuses to run while it is set.
- A law enforced by tests needs a `bite.Off("<law>")` call in non-test Go code of its module, or a `Bites: not yet. <why>` gap.
- A bitten script must exit 1 and print `lawgate: rejects <law>`; any other failure is an infrastructure error.
- A Node bite on a direct `node --test` command reruns only the tests whose name contains the enforcer's (`--test-name-pattern`); any other Node runner reruns its whole suite.
- When that pattern skips the enforcer (Node skips a `test()` nested in a parent whose name does not match), the rerun falls back to the whole test command. A rerun that still has no result for the named test is reported as "did not run", with or without the pattern.
- `LAWS.md` refuses a law whose name starts with the reserved `lawgate/` prefix, and reads no enforcer for it.
- A lawgate binary without Go build information reports that once as an infrastructure error and checks no Go bite, instead of reporting every hook as missing.
- The bite phase lists a module's tagged import closure once and scans each test package's hooks once, however many laws its tests enforce.
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
