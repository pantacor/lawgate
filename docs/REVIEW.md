# Review

`lawgate review` shows what governs a change while you review it: the
declared machine state and site a line belongs to, the laws that cover it and
whether they hold, the law a function is the subject of or a test proves, and
the commit that explains why the line changed. It reads the same inputs as
`lawgate check` (`lawgate.json`, every `LAWS.md`, each module's machine
command) and works in any repository that has a `lawgate.json`. It never
imports adopter code.

It has two faces: a branch report (Markdown or JSON) for a terminal or a pull
or merge request, and a language server for editors.

## The branch report

```bash
lawgate review                       # Markdown, against the default branch
lawgate review --base main           # against another branch or commit
lawgate review --base main --json    # JSON for tools
lawgate review --index               # the review index itself, as JSON
```

| Flag | What it does |
| --- | --- |
| `--base REF` | The branch or commit the changes are reviewed against. Default: the remote's default branch (`git symbolic-ref refs/remotes/origin/HEAD`), else the first of `origin/main`, `main`, `origin/master`, `master` that exists. |
| `--root DIR` | Where to start looking for `lawgate.json` (default: the current directory). lawgate walks up to the nearest `lawgate.json`. |
| `--json` | Print the report as JSON instead of Markdown. |
| `--index` | Print the repository's review index (states, sites, laws, subjects, tests, problems) as JSON. |
| `--blob-url URL` | Link files in the Markdown under `URL/<path>#L<line>`, for example `https://github.com/org/repo/blob/$SHA`. |
| `--pr` | Ask the platform's CLI which pull or merge request brought each commit: `gh` when `lawgate.json` says `github`, `glab` for `gitlab`. Off by default (it needs the network); a missing or logged-out CLI is skipped. |

Run from a directory below the repository root, the report covers that
directory only (`cd example && lawgate review --base main`).

The report compares the merge base of `--base` and `HEAD` with the working
tree: committed changes, uncommitted changes, and files git does not track
yet. It lists:

- **Laws this branch touches**, broken ones first, then laws whose text
  changed, references to laws no `LAWS.md` writes, pending laws, and the
  rest; each with its verdict on every machine and the places the branch
  touches it.
- **Rule changes**: the commits that touch a rule file, as `rulechanges`
  classifies them (the same list `lawgate rule-files` prints), and whether
  each carries a `Rule-change-approved-by:` trailer or a later commit's
  `Rule-change-approves:`. The trailer is an audit note: the approval is
  the owner's approval on the pull request or merge request, which
  `lawgate rule-changes` checks in CI when `require_approval` is on.
- **Proof lock**: when the branch changes `LAWS.lock`, which laws' proofs or
  subjects changed. Each needs the owner's review.
- **Declaration problems**: drift between a machine and its source (a tag no
  site claims, a site whose tag is gone), a machine command that failed, a
  subject or test that is not in the code.
- **Changes**, file by file: each changed hunk with the states whose code it
  touches, the sites on its lines, the site declarations it edits, the
  subjects and enforcing tests whose functions it changes, the laws all of
  those bring, the `LAWS.md` sections whose text it changes, and the commits
  that changed those lines (or "not committed yet"). A hunk nothing governs
  is only counted.

The Markdown starts with `<!-- lawgate-review -->`, so a CI job can find its
own comment on a pull or merge request and update it instead of adding one.

## How the index is built

For every module of `lawgate.json`:

1. Its `LAWS.md` and the shared documents it lists are read with `lawsdoc`:
   each law's name, text, line range, `Source`, `Enforced by`, `Status` and
   `Subject` fields. A law name two modules use is kept twice; the later one
   is keyed `<LAWS.md path>#<name>`.
2. A Go module with `"machines": true` runs its machine command the way
   `lawgate check` does (same command, directory, environment, timeout and
   refusal of symlinked inputs), with `-index FILE` in place of
   `-laws -junit FILE`. The runner writes a versioned JSON document
   (package `review/index`, `"version": 1`): every machine with its states,
   its sites resolved to source lines by `resolve` and to their `Tag:` or
   `Match:` declaration, its laws' verdicts and its drift. `runner.Main`
   answers `-index` already; a custom machine command must too.
3. Each law's subjects (`- Subject:`) and enforcing tests (`test` in
   `- Enforced by:`) are placed on the lines of their functions with
   `go/parser`: subjects through the repository's `go.mod` files, tests
   through the active Go build, as `lawgate check` finds them.

What cannot be read becomes a problem in the index; only an unreadable
`lawgate.json` stops the review. Subjects and tests are resolved for Go
modules; the machines of a Node module are not indexed.

## In an editor

`lawgate review -lsp` serves a language server on standard input and
output (`--stdio` is accepted too, as some clients add it; `--root DIR`
names the repository, otherwise it is found from the client's workspace
folders). The server builds the index once at start, and again one second
after a `.go` file or a `LAWS.md` is saved, so a changed declaration shows
up without a restart.

- **Code lenses** on every state's function
  (`⚙ state worker/load · 1 site · ✅ no-silent-errors`), every site and site
  declaration (`◆ fault 1 · load/load-job`), every subject
  (`🛡 protected by law rejects-empty-payload`), every enforcing test
  (`🧪 proves law rejects-empty-payload`), and each law's verdict on its
  `LAWS.md` heading.
- **Hover** on a site, a declaration, a state's function, a subject or a
  test: what it is, the laws that cover it with their verdicts, and why the
  line last changed (commit, body, approval trailer). Hover on a law name
  anywhere shows the law.
- **Go to definition** from a site to its declaration and back, from a law
  name to its `LAWS.md` section, from a subject or test to its law, and from
  a subject or test named in `LAWS.md` to its function.
- **Diagnostics**: declaration problems on their line; a broken or pending
  law, and a subject or test that cannot be found, on the law's heading; a
  `law <name>` reference to a law no `LAWS.md` writes.

### Commands

The server answers `workspace/executeCommand` with these commands. Their
ids are a contract with the editor extensions.

| Command | Arguments | Answer |
| --- | --- | --- |
| `lawgate.review` | `[base, lookUpRequests]` | the report, as JSON |
| `lawgate.reviewMarkdown` | `[base, lookUpRequests]` | the report, as Markdown |
| `lawgate.why` | `[uri, line (0-based), lookUpRequests]` | the commit that last changed the line |
| `lawgate.reindex` | `[]` | rebuilds the index; its summary |
| `lawgate.summary` | `[]` | the index's summary: sites, states, law verdicts (holds, broken, pending), problems |
| `lawgate.index` | `[]` | the whole index |

An empty `base` means the default branch. `lookUpRequests` (default
`false`) asks `gh` or `glab` for the pull or merge request.

The code lenses name client commands an extension carries out:
`lawgate.showLaw [law key]`, `lawgate.showSite [machine, state, step, tag]`,
`lawgate.showState [machine, state]`, `lawgate.showSubject [law key]` (from
a subject to the tests that prove its law) and `lawgate.showTest [law key]`
(from a test to its law's subjects). A client without them still shows the
titles. After each new index the server sends `workspace/codeLens/refresh`
and a `lawgate/indexChanged` notification carrying the summary.

### VS Code

The extension in [`editors/vscode`](../editors/vscode/) starts the server
and adds the branch review view, notes on changed lines, the report panel,
"copy as Markdown", "why did this line change?" and a status bar verdict.
See its README for install and settings.

### Neovim (0.11+)

```lua
vim.lsp.config("lawgate", {
  cmd = { "lawgate", "review", "-lsp" },
  filetypes = { "go", "markdown" },
  root_markers = { "lawgate.json" },
})
vim.lsp.enable("lawgate")

-- :LawgateReview [base] opens the branch report in a split.
vim.api.nvim_create_user_command("LawgateReview", function(o)
  local client = vim.lsp.get_clients({ name = "lawgate" })[1]
  client:exec_cmd({ title = "review", command = "lawgate.reviewMarkdown",
    arguments = { o.args } }, {}, function(err, md)
    if err then return vim.notify(err.message, vim.log.levels.ERROR) end
    vim.cmd("vnew")
    vim.bo.filetype = "markdown"
    vim.api.nvim_buf_set_lines(0, 0, -1, false, vim.split(md, "\n"))
  end)
end, { nargs = "?" })
```

Without `lawgate` on `PATH`, use
`{ "go", "run", "github.com/pantacor/lawgate/cmd/lawgate", "review", "-lsp" }`
from a module that requires lawgate: it runs the version the module pins.

### Helix

```toml
# .helix/languages.toml
[language-server.lawgate]
command = "lawgate"
args = ["review", "-lsp"]

[[language]]
name = "go"
language-servers = ["gopls", "lawgate"]

[[language]]
name = "markdown"
language-servers = ["lawgate"]
```
