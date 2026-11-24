# Lawgate for VS Code

Review a branch with the laws, proofs and state machines that `lawgate check`
enforces. For the line you are reading, the extension shows which law protects
it, which test proves that law, whether the law holds, and the commit that
explains why the line changed.

It works in any repository that has a `lawgate.json`. It is a thin client: the
analysis is done by `lawgate review -lsp`, a language server over stdio, and
the editor only shows its answers. Nothing leaves your machine, except the
optional pull or merge request lookup (see `lawgate.mergeRequests`).

## Install

Build the package and install it:

```sh
cd editors/vscode
npm ci
npm run package        # writes lawgate.vsix
code --install-extension lawgate.vsix
```

The extension activates in a workspace that contains a `lawgate.json`.

## The server

The extension starts the language server in this order:

1. The binary named by the `lawgate.serverPath` setting.
2. `lawgate` from `PATH` (`go install github.com/pantacor/lawgate/cmd/lawgate@v0.1.0`).
3. `go run github.com/pantacor/lawgate/cmd/lawgate review -lsp`, run from the
   workspace folder. This uses the version your module pins in `go.mod`, so the
   server always matches the repository. It needs a Go toolchain and takes a
   moment on the first start.

When `lawgate.json` is in a sub-folder of the workspace, the server is started
with `-root <that folder>`. Paths the server returns are relative to the folder
that holds `lawgate.json`.

Saving a `.go` file or a `LAWS.md` re-checks the laws about a second later.
"Lawgate: Restart language server" restarts it.

## Features

- **Code lenses.** On every state's function, every declared machine site, each
  law's verdict on its `LAWS.md` heading, "protected by law X" on a subject
  function and "proves law X" on a test function.
- **Hover** on a site, a state, a subject, a test or a law name: the law's
  text, its verdict and why the line last changed.
- **Go to definition** from a site to its declaration and back, from a law name
  to its `LAWS.md` section, and between a subject, its tests and the law.
- **Diagnostics** for declaration drift, broken or pending laws, and
  references to a law no `LAWS.md` writes down.
- **Branch review view** (Lawgate in the activity bar). The laws the branch
  touches with their verdicts, subjects and proving tests; rule changes and
  whether each has a `Rule-change-approved-by` trailer; `LAWS.lock` changes,
  which need the owner's review; declaration drift; and every governed change
  with its states, sites, subjects, tests, laws and commits. A click opens the
  diff against the merge base.
- **Notes on changed lines.** After a review, every changed hunk that a state,
  site, subject, test or law governs gets a read-only note, in editors and in
  diffs.
- **Report panel** and **Copy review report as Markdown**, for a pull or merge
  request description.
- **Why did this line change?** in the editor context menu: the commit, its
  author and body, and the request that brought it when the lookup is on.
- **Status bar.** Laws that hold out of the laws checked, and the count of broken
  laws and drift. Click it to review the branch.

## Commands

| Command | What it does |
|---|---|
| `Lawgate: Review branch` | Reviews the branch against the base and fills the view |
| `Lawgate: Set review base...` | Chooses the branch or commit to review against (kept per workspace) |
| `Lawgate: Open review report` | Shows the report in a panel |
| `Lawgate: Copy review report as Markdown` | Copies the report |
| `Lawgate: Toggle notes on changed lines` | Shows or hides the notes |
| `Lawgate: Why did this line change?` | Explains the line under the cursor |
| `Lawgate: Re-check machines and laws` | Rebuilds the index now |
| `Lawgate: Restart language server` | Restarts the server |

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `lawgate.base` | empty | The branch to review against. Empty means the repository's default branch (`git symbolic-ref refs/remotes/origin/HEAD`), else `origin/main`. |
| `lawgate.serverPath` | empty | A `lawgate` binary. Empty means `lawgate` on `PATH`, else `go run`. |
| `lawgate.siteLenses` | `true` | A code lens on every declared site. |
| `lawgate.notesOnChangedLines` | `true` | Notes on governed hunks after a review. |
| `lawgate.mergeRequests` | `false` | Ask `gh` (GitHub) or `glab` (GitLab), when installed and logged in, which pull or merge request brought each commit. |

Changing `lawgate.serverPath` or `lawgate.siteLenses` restarts the server.

## What the server must answer

For tools that implement the server side, the extension calls
`workspace/executeCommand` with these ids: `lawgate.review [base,
mergeRequests]`, `lawgate.reviewMarkdown [base, mergeRequests]`,
`lawgate.why [uri, line]`, `lawgate.reindex`, `lawgate.summary` and
`lawgate.index`. It listens for the `lawgate/indexChanged` notification. The
code lenses run these client commands: `lawgate.showLaw [law]`,
`lawgate.showSite [machine, state, step, name]`,
`lawgate.showState [machine, state]`, `lawgate.showSubject [law]` and
`lawgate.showTest [law]`.

## Develop

```sh
npm ci
npm run check     # type-check
npm run package   # type-check, bundle, write lawgate.vsix
```

`out/`, `node_modules/` and `lawgate.vsix` are not committed.

## Licence

MIT, as the repository (`LICENSE`).
