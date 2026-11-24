import * as path from "path";
import * as vscode from "vscode";
import { showLaw, showLawCode, showSite, showState } from "./lenses";
import { Notes } from "./notes";
import { commitMarkdown, openHunk, openLocation, showRuleChange, showRuleFile } from "./open";
import { showMarkdown } from "./panel";
import { config, defaultBase, Server } from "./server";
import { ReviewTree } from "./tree";
import { Commit, Index, Report, RuleChange, Summary } from "./types";

interface Roots {
  workspaceDir: string;
  lawgateDir: string;
}

// findRoots locates the directory that holds lawgate.json: a workspace
// folder itself, else the nearest one below it.
async function findRoots(): Promise<Roots | undefined> {
  for (const folder of vscode.workspace.workspaceFolders ?? []) {
    const dir = folder.uri.fsPath;
    const own = await vscode.workspace.findFiles(new vscode.RelativePattern(folder, "lawgate.json"), null, 1);
    if (own.length > 0) return { workspaceDir: dir, lawgateDir: dir };
  }
  const found = await vscode.workspace.findFiles("**/lawgate.json", "**/{node_modules,vendor,testdata}/**", 20);
  const nearest = found.map((u) => u.fsPath).sort((a, b) => a.length - b.length)[0];
  if (!nearest) return undefined;
  const folder = vscode.workspace.getWorkspaceFolder(vscode.Uri.file(nearest));
  return { workspaceDir: folder?.uri.fsPath ?? path.dirname(nearest), lawgateDir: path.dirname(nearest) };
}

function updateStatus(status: vscode.StatusBarItem, s: Summary): void {
  const bad = s.broken > 0 || s.drift > 0;
  const broken = s.broken ? ` · ${s.broken} broken` : "";
  const drift = s.drift ? ` · ${s.drift} drift` : "";
  status.text = `$(law) ${bad ? "$(error) " : ""}${s.holds}/${s.laws} laws${broken}${drift}`;
  status.tooltip = `Lawgate: ${s.states} states, ${s.sites} sites, ${s.laws} law checks (${s.holds} hold, ${s.broken} broken, ${s.pending} pending), ${s.drift} declaration drifts.\nClick to review the branch.`;
  status.backgroundColor = bad ? new vscode.ThemeColor("statusBarItem.errorBackground") : undefined;
}

// Session holds what the commands share: the server, the last report and the
// index the code lenses look targets up in.
class Session {
  report: Report | undefined;
  private cachedIndex: Index | undefined;
  readonly tree: ReviewTree;
  readonly notes = new Notes();
  notesOn = config().get<boolean>("notesOnChangedLines", true);
  readonly server: Server;

  constructor(
    readonly ctx: vscode.ExtensionContext,
    readonly roots: Roots,
    readonly status: vscode.StatusBarItem,
  ) {
    this.tree = new ReviewTree(() => roots.lawgateDir);
    this.server = new Server(roots.workspaceDir, roots.lawgateDir, {
      onIndexChanged: (s) => {
        this.cachedIndex = undefined;
        updateStatus(status, s);
      },
    });
  }

  get root(): string {
    return this.roots.lawgateDir;
  }

  async index(): Promise<Index> {
    this.cachedIndex ??= await this.server.exec<Index>("lawgate.index");
    return this.cachedIndex;
  }

  async base(): Promise<string> {
    const chosen = this.ctx.workspaceState.get<string>("lawgate.base") || config().get<string>("base", "");
    return chosen || defaultBase(this.root);
  }

  async refresh(): Promise<void> {
    this.tree.setLoading();
    try {
      const base = await this.base();
      this.report = await vscode.window.withProgress({ location: { viewId: "lawgate.changes" }, title: "Reviewing the branch" }, () =>
        this.server.exec<Report>("lawgate.review", base, config().get<boolean>("mergeRequests", false)),
      );
      this.tree.setReport(this.report);
      if (this.notesOn) this.notes.show(this.root, this.report);
    } catch (e) {
      this.tree.setReport(undefined, String(e instanceof Error ? e.message : e));
    }
  }

  async markdown(): Promise<[string, string]> {
    const base = await this.base();
    const md = await this.server.exec<string>("lawgate.reviewMarkdown", base, config().get<boolean>("mergeRequests", false));
    return [base, md];
  }
}

function registerReviewCommands(s: Session, reg: Register): void {
  reg("lawgate.refresh", () => s.refresh());
  reg("lawgate.setBase", async () => {
    const value = await vscode.window.showInputBox({ prompt: "Review against which branch or commit?", value: await s.base() });
    if (value === undefined) return;
    await s.ctx.workspaceState.update("lawgate.base", value || undefined);
    await s.refresh();
  });
  reg("lawgate.openReport", async () => {
    const [base, md] = await s.markdown();
    await showMarkdown(`Lawgate review · ${base}`, md);
  });
  reg("lawgate.copyReport", async () => {
    const [, md] = await s.markdown();
    await vscode.env.clipboard.writeText(md);
    void vscode.window.showInformationMessage("Lawgate review copied as Markdown: paste it into the pull or merge request.");
  });
  reg("lawgate.toggleNotes", () => {
    s.notesOn = !s.notesOn;
    if (s.notesOn && s.report) s.notes.show(s.root, s.report);
    else s.notes.clear();
    void vscode.window.setStatusBarMessage(`Lawgate notes ${s.notesOn ? "on" : "off"}`, 2000);
  });
  reg("lawgate.reindex", async () => updateStatus(s.status, await s.server.exec<Summary>("lawgate.reindex")));
  reg("lawgate.restartServer", () => s.server.restart());
}

async function whyLine(s: Session): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return;
  const c = await s.server.exec<Commit>("lawgate.why", editor.document.uri.toString(), editor.selection.active.line);
  if (c.uncommitted) {
    void vscode.window.showInformationMessage("This line is not committed yet.");
    return;
  }
  const actions = ["Show commit", ...(c.merge_request ? ["Open merge request"] : [])];
  const pick = await vscode.window.showInformationMessage(`${c.short} ${c.subject} — ${c.author}, ${c.date}`, ...actions);
  if (pick === "Show commit") await showMarkdown(`${c.short} ${c.subject}`, commitMarkdown(c));
  if (pick === "Open merge request" && c.merge_request) await vscode.env.openExternal(vscode.Uri.parse(c.merge_request));
}

// The commands the tree, the notes and the server's code lenses run.
function registerNavigation(s: Session, reg: Register): void {
  reg("lawgate.whyLine", () => whyLine(s));
  reg("lawgate.openLocation", (file: string, line: number) => openLocation(s.root, file, line));
  reg("lawgate.openHunk", (file: string, line: number) => openHunk(s.root, s.report?.merge_base, file, line));
  reg("lawgate.showRuleChange", (rc: RuleChange) => showRuleChange(s.root, rc));
  reg("lawgate.showRuleFile", (rc: RuleChange, file: string) => showRuleFile(s.root, rc, file));
  reg("lawgate.showCommit", (c: Commit) => showMarkdown(`${c.short} ${c.subject}`, commitMarkdown(c)));
  reg("lawgate.showLaw", async (name: string) => showLaw(s.root, await s.index(), name));
  reg("lawgate.showSite", async (machine: string, state: string, step: string, name: string) => showSite(s.root, await s.index(), machine, state, step, name));
  reg("lawgate.showState", async (machine: string, id: string) => showState(s.root, await s.index(), machine, id));
  reg("lawgate.showSubject", async (law: string) => showLawCode(s.root, await s.index(), law, "subject"));
  reg("lawgate.showTest", async (law: string) => showLawCode(s.root, await s.index(), law, "test"));
}

type Register = (id: string, fn: (...a: any[]) => unknown) => void;

async function startServer(s: Session): Promise<void> {
  try {
    await s.server.start();
  } catch (e) {
    const why = e instanceof Error ? e.message : String(e);
    void vscode.window.showErrorMessage(`Lawgate: the language server did not start (${why}). Set lawgate.serverPath, or install lawgate.`);
  }
}

// A changed server setting restarts the server, which reads them at start.
function restartOnSettings(s: Session): vscode.Disposable {
  return vscode.workspace.onDidChangeConfiguration(async (e) => {
    if (!e.affectsConfiguration("lawgate.serverPath") && !e.affectsConfiguration("lawgate.siteLenses")) return;
    await s.server.stop();
    await startServer(s);
  });
}

export async function activate(ctx: vscode.ExtensionContext): Promise<void> {
  const roots = await findRoots();
  if (!roots) return;

  const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
  status.command = "lawgate.refresh";
  status.text = "$(law) Lawgate…";
  status.show();

  const s = new Session(ctx, roots, status);
  const reg: Register = (id, fn) => ctx.subscriptions.push(vscode.commands.registerCommand(id, fn));
  ctx.subscriptions.push(status, s.notes, vscode.window.registerTreeDataProvider("lawgate.changes", s.tree), restartOnSettings(s));
  registerReviewCommands(s, reg);
  registerNavigation(s, reg);
  ctx.subscriptions.push({ dispose: () => void s.server.stop() });
  await startServer(s);
}

export function deactivate(): void {}
