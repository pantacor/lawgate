# Developer Guide

This guide is for work on lawgate itself. To use lawgate in your own
repository, read the [User Guide](USER_GUIDE.md).

## Requirements

- Go 1.25 or later. lawgate imports only the standard library.
- Git, for the ownership and audit commands and their tests.
- Node.js, for the tests that run Node enforcers.
- Hugo, only to build the website.

## Layout

The module is `github.com/pantacor/lawgate`. Each directory is one package.

| Package | What it does |
| --- | --- |
| `cmd/lawgate` | The command line: `init`, `check`, `readability-baseline`, `rule-files`, `codeowners`, `check-codeowners`, `rule-changes`. |
| `inventory` | Reads `lawgate.json` and parses the commands it lists. |
| `lawsdoc` | Reads a `LAWS.md` and checks it against the code (layer 1). |
| `machine` | A worker's control flow as data, laws over the graph, and witnesses. |
| `resolve` | Binds the declared sites of a machine to its Go source (layer 2). |
| `gen` | Writes Go types for a machine's states and checks they are fresh (layer 3). |
| `run` | Records the moves of one pass and refuses an undeclared one (layer 3). |
| `prove` | Checks a kernel on every input inside declared bounds (layer 4). |
| `runner` | The shared `cmd/machine` entry point for adopting Go modules. |
| `bite` | The `LAWGATE_BITE` variable name and the reserved `lawgate/` prefix, for Node and script reruns. |
| `mutate` | Finds a law's subject and generates its mutants. |
| `lawslock` | Fingerprints proofs and subjects, and reads and writes `LAWS.lock`. |
| `readability` | The five readability rules and the baseline ratchet. |
| `rulechanges` | Finds rule files, writes and checks CODEOWNERS, audits Git history. |
| `gosource` | Finds Go modules, source files and machine declarations. |
| `report` | Writes and reads JUnit, and writes the Markdown and HTML reports. |
| `templates` | The CI and CODEOWNERS templates that `init` writes. |

Other directories:

| Path | What it holds |
| --- | --- |
| `example/` | A small adopting module with its own `go.mod`. The gate checks it end to end. |
| `LAWS.md` | lawgate's own laws. |
| `lawgate.json` | lawgate's own inventory: the root module and `example`. |
| `.lawgate-readability` | lawgate's readability baseline. It is empty. |
| `docs/` | These guides. |
| `site/` | The Hugo website, which renders `docs/` in place. |

## Run the tests

```bash
gofmt -l .
go vet ./...
go test -race -count=1 ./...
```

CI runs these three commands. `gofmt -l .` must print nothing.

`example/` is a separate module, so `go test ./...` at the root does not run
its tests. Run them from inside it, or run the gate:

```bash
(cd example && go test ./...)
```

## Run lawgate's own gate

```bash
go run ./cmd/lawgate check
```

This checks lawgate with lawgate. It runs the tests of both modules, checks
the `worker` machine of the example, checks both law documents, mutates each
law's subject to show that its tests bite, compares the proofs with
`LAWS.lock`, and checks readability and CODEOWNERS. The
reports go to a new directory under `laws-report/`. Use `--out DIR` to put
them somewhere else.

Run the gate before every commit. If it fails, fix the code, not the law.

## The self-check

lawgate must keep the rules it gives to others. Three things make sure of it.

**Its laws are in `LAWS.md`.** Each one names the tests that enforce it. For
example `laws-bite` is enforced by `TestCheckLawsRequiresABreakingWitness`
and `TestBiteRunsAgreedEnforcers`. The gate checks that these tests exist,
name the law and ran.

**Its own enforcers bite.** Each law in `LAWS.md` names its subject, such as
`github.com/pantacor/lawgate/run.cursor.checkContinuity`. The gate mutates
the subject and expects the law's tests to fail. lawgate's Go code has no
test switch. The proofs are pinned in `LAWS.lock`, and CI replays the base
commit's proofs with `--base`.

**Its code passes the readability rules.** `TestCoreFollowsTheReadabilityRule`
runs the checker over the whole repository and fails on any finding. It first
checks that the checker still reports a known violation, so that zero
findings mean something. The baseline is empty and must stay empty.

In CI, the `review-rules` job builds the verifier from the base commit, not
from the change. A change cannot weaken the checks that judge it. A change to
`lawgate.json` that the base verifier cannot read must land in two steps.

## The readability rules

All Go code in this repository follows these rules, tests included:

1. Inside a function body, no statement sits more than 2 block levels deep.
   A single guard at the start of a loop body, such as
   `if skip { continue }`, does not add a level.
2. No struct or map literal is written inline inside another one. Build the
   inner value first and give it a name.
3. No function body is longer than 40 lines.
4. No condition has 3 or more `&&` or `||` operands. Move it to a named
   variable or function.
5. No function takes a `bool` parameter.

A finding looks like this:

```
self-check violation [nesting] cmd/lawgate/check.go:120 (checkCommand): cmd/lawgate/check.go:120: checkCommand nested 3 levels deep (max 2)
```

Fix the code. Do not add the file to the baseline.

## Change a rule

These files are rule files: `LAWS.md`, `example/LAWS.md`, `lawgate.json`,
`.lawgate-readability`, `.github/CODEOWNERS`, the CI workflows, the
templates, and the Go packages that declare or enforce a law. The owner
reviews every change to one.

- Do not add, remove, weaken or reword a law without the owner's approval.
- A law comes from a document or a dated owner decision. Do not invent one.
- When you add a file that becomes a rule file, regenerate CODEOWNERS:

```bash
go run ./cmd/lawgate codeowners --fallback > .github/CODEOWNERS
go run ./cmd/lawgate check-codeowners
```

`go run ./cmd/lawgate rule-files` lists the current rule files.

## Add a check

1. Write the behaviour and its test in the package that owns it.
2. If the test enforces a law, name the law in the test's doc comment.
3. Ask the owner to add the law's `Subject:` line to `LAWS.md`. Keep the
   subject a small function that holds the law's guard.
4. Run `go run ./cmd/lawgate check`. If a mutant survives, add the missing
   case to the test. Run `go run ./cmd/lawgate lock` when a proof or a
   subject changed; the owner reviews `LAWS.lock`.
5. Add a line to `CHANGELOG.md`.

An error message should say what is wrong and what to do. Users read these
messages in the summary, and [Troubleshooting](TROUBLESHOOTING.md) quotes
them. When you change a message, update that page.

## Build the website

The site is in `site/`. It mounts `docs/` as content, so the guides have one
copy.

```bash
cd site
hugo server          # preview at http://localhost:1313/lawgate/
hugo --gc --minify   # build into site/public
```

The workflow `.github/workflows/pages.yml` builds the site on each pull
request that touches `site/` or `docs/`, and publishes it to GitHub Pages on
each push to `main`.

Links between guides use the file name, such as `USER_GUIDE.md`. Links to the
rest of the repository start with `../`. The site rewrites both.
