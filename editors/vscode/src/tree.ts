import * as path from "path";
import * as vscode from "vscode";
import { commitMarkdown } from "./open";
import { CodeRef, Commit, FileReport, HunkReport, LawImpact, Report, RuleChange, governs, lawStatusIcon, refLabel, statusIcon } from "./types";

type Detail = { kind: "detail"; label: string; description?: string; tooltip?: string; icon: string; command?: vscode.Command };

type Node =
  | { kind: "group"; id: string; label: string; description?: string; children: () => Node[]; icon?: string }
  | { kind: "law"; law: LawImpact }
  | { kind: "rule"; rule: RuleChange }
  | { kind: "ruleFile"; rule: RuleChange; file: string }
  | { kind: "where"; label: string; file: string; line: number }
  | { kind: "file"; file: FileReport; hunks: HunkReport[] }
  | { kind: "hunk"; file: FileReport; hunk: HunkReport }
  | Detail
  | { kind: "message"; label: string; icon?: string };

function open(file: string, line: number): vscode.Command {
  return { command: "lawgate.openLocation", title: "Open", arguments: [file, line] };
}

function lawDescription(l: LawImpact): string {
  const bits: string[] = [];
  if (!l.known) bits.push("not written in a LAWS.md");
  else bits.push(l.status === "unchecked" ? "tests only" : l.status);
  if (l.text_changed) bits.push("law text changed");
  if (l.parent) bits.push(`under ${l.parent}`);
  return bits.join(" · ");
}

function lawTooltip(l: LawImpact): vscode.MarkdownString {
  const verdicts = (l.verdicts ?? []).map((v) => `- ${statusIcon(v.status)} ${v.status} on \`${v.machine}\`${v.pending ? ` (pending: ${v.pending})` : ""}`);
  return new vscode.MarkdownString([`**${l.name}**${l.title ? ` — ${l.title}` : ""}`, ...verdicts].join("\n\n"));
}

function lawItem(l: LawImpact): vscode.TreeItem {
  const it = new vscode.TreeItem(`${lawStatusIcon(l)} ${l.name}`, vscode.TreeItemCollapsibleState.Collapsed);
  it.description = lawDescription(l);
  it.tooltip = lawTooltip(l);
  if (l.file && l.line) it.command = open(l.file, l.line);
  it.contextValue = "law";
  return it;
}

function ruleItem(rc: RuleChange, commit: Commit | undefined): vscode.TreeItem {
  const it = new vscode.TreeItem(`${rc.commit} ${rc.subject}`, vscode.TreeItemCollapsibleState.Collapsed);
  it.description = rc.approved ? `approved by ${(rc.approved_by ?? ["a later commit"]).join(", ")}` : "no Rule-change-approved-by trailer";
  const head = commit ? commitMarkdown(commit) : `**${rc.commit}** ${rc.subject}`;
  it.tooltip = new vscode.MarkdownString([head, "Rule files:", ...rc.files.map((f) => `- \`${f}\``)].join("\n\n"));
  it.iconPath = new vscode.ThemeIcon(rc.approved ? "pass" : "error");
  it.command = { command: "lawgate.showRuleChange", title: "Show the rule change", arguments: [rc] };
  return it;
}

function fileDescription(f: FileReport, governed: number): string {
  const parts = [`${path.dirname(f.path)}`, `${governed}/${f.hunks?.length ?? 0} governed`];
  if (f.rule_file) parts.push("rule file");
  if (f.lock_file) parts.push("LAWS.lock: proof or subject changed, needs the owner's review");
  if (f.status !== "modified") parts.push(f.status);
  return parts.join(" · ");
}

function hunkItem(file: FileReport, h: HunkReport): vscode.TreeItem {
  const it = new vscode.TreeItem(`L${h.new_start}${h.header ? ` · ${h.header.trim()}` : ""}`, vscode.TreeItemCollapsibleState.Collapsed);
  it.description = [...(h.states ?? []), `+${h.added?.length ?? 0} −${h.removed?.length ?? 0}`].join(" · ");
  const lawful = (h.laws?.length ?? 0) + (h.law_text?.length ?? 0) > 0;
  it.iconPath = new vscode.ThemeIcon(lawful ? "law" : "symbol-event");
  it.command = { command: "lawgate.openHunk", title: "Open change", arguments: [file.path, h.new_start] };
  return it;
}

function detailItem(n: Detail): vscode.TreeItem {
  const it = new vscode.TreeItem(n.label);
  it.description = n.description;
  it.tooltip = n.tooltip ? new vscode.MarkdownString(n.tooltip) : undefined;
  it.iconPath = new vscode.ThemeIcon(n.icon);
  it.command = n.command;
  return it;
}

function whereNode(w: string): Node {
  const m = /^(.*?):(\d+)(.*)$/.exec(w);
  return m ? { kind: "where", label: w, file: m[1], line: Number(m[2]) } : { kind: "message", label: w };
}

function refDetail(r: CodeRef, role: string, icon: string): Detail {
  const law = r.law ? ` · law ${r.law}` : "";
  return { kind: "detail", label: refLabel(r), description: `${role}${law}`, icon, command: open(r.file, r.line) };
}

// Where a law is protected and proved, next to the places it touches.
function lawChildren(l: LawImpact): Node[] {
  const subjects = (l.subjects ?? []).map((r) => refDetail(r, "subject", "shield"));
  const tests = (l.tests ?? []).map((r) => refDetail(r, "test", "beaker"));
  return [...l.where.map(whereNode), ...subjects, ...tests];
}

function siteDetails(h: HunkReport): Node[] {
  const sites = (h.sites ?? []).map((s): Node => {
    const faulty = s.kind === "fail" || s.kind === "fault";
    const command = s.decl_file ? open(s.decl_file, s.decl_line ?? 1) : undefined;
    return { kind: "detail", label: `${s.tag ?? s.step} · ${s.kind}`, description: `${s.state}/${s.step}`, tooltip: s.effect, icon: faulty ? "error" : "symbol-event", command };
  });
  const declarations = (h.declarations ?? []).map((s): Node => {
    return { kind: "detail", label: `declaration of ${s.tag ?? s.step}`, description: `${s.state}/${s.step}`, tooltip: s.effect, icon: "symbol-field", command: open(s.file, s.line) };
  });
  return [...sites, ...declarations];
}

function lawDetails(h: HunkReport, laws: LawImpact[]): Node[] {
  const named = (h.laws ?? []).map((n): Node => {
    const l = laws.find((x) => x.name === n);
    const command = l?.file && l.line ? open(l.file, l.line) : undefined;
    return { kind: "detail", label: `${lawStatusIcon(l)} ${n}`, description: "law", icon: "law", command };
  });
  const changed = (h.law_text ?? []).map((n): Node => ({ kind: "detail", label: n, description: "law text changed", icon: "edit" }));
  const removed = (h.removed_tags ?? []).map((t): Node => ({ kind: "detail", label: t, description: "log tag removed", icon: "trash" }));
  return [...named, ...changed, ...removed];
}

function commitDetails(h: HunkReport, commits: Commit[]): Node[] {
  const out = (h.commits ?? []).map((short): Node => {
    const c = commits.find((x) => x.short === short);
    const command = c ? { command: "lawgate.showCommit", title: "Show commit", arguments: [c] } : undefined;
    return { kind: "detail", label: c?.subject ?? short, description: `why · ${short}`, tooltip: c ? commitMarkdown(c) : undefined, icon: "git-commit", command };
  });
  if (h.uncommitted) out.push({ kind: "detail", label: "not committed yet", description: "why", icon: "circle-outline" });
  return out;
}

function hunkDetails(h: HunkReport, r: Report | undefined): Node[] {
  const states = (h.states ?? []).map((s): Node => ({ kind: "detail", label: s, description: "state", icon: "symbol-enum" }));
  const subjects = (h.subjects ?? []).map((s) => refDetail(s, "protected subject", "shield"));
  const tests = (h.tests ?? []).map((s) => refDetail(s, "proof", "beaker"));
  return [...states, ...siteDetails(h), ...subjects, ...tests, ...lawDetails(h, r?.laws ?? []), ...commitDetails(h, r?.commits ?? [])];
}

export class ReviewTree implements vscode.TreeDataProvider<Node> {
  private report: Report | undefined;
  private error: string | undefined;
  private loading = false;
  private readonly changed = new vscode.EventEmitter<Node | undefined>();
  readonly onDidChangeTreeData = this.changed.event;

  constructor(private readonly root: () => string | undefined) {}

  setLoading(): void {
    this.loading = true;
    this.error = undefined;
    this.changed.fire(undefined);
  }

  setReport(r: Report | undefined, error?: string): void {
    this.loading = false;
    this.report = r;
    this.error = error;
    this.changed.fire(undefined);
  }

  getTreeItem(n: Node): vscode.TreeItem {
    switch (n.kind) {
      case "group":
        return this.groupItem(n);
      case "law":
        return lawItem(n.law);
      case "rule":
        return ruleItem(n.rule, this.report?.commits?.find((x) => x.short === n.rule.commit));
      case "ruleFile":
        return this.ruleFileItem(n.rule, n.file);
      case "where":
        return this.whereItem(n);
      case "file":
        return this.fileItem(n.file, n.hunks.length);
      case "hunk":
        return hunkItem(n.file, n.hunk);
      case "detail":
        return detailItem(n);
      case "message":
        return this.messageItem(n);
    }
  }

  getChildren(n?: Node): Node[] {
    if (!n) return this.roots();
    switch (n.kind) {
      case "group":
        return n.children();
      case "law":
        return lawChildren(n.law);
      case "rule":
        return n.rule.files.map((file): Node => ({ kind: "ruleFile", rule: n.rule, file }));
      case "file":
        return n.hunks.map((h): Node => ({ kind: "hunk", file: n.file, hunk: h }));
      case "hunk":
        return hunkDetails(n.hunk, this.report);
    }
    return [];
  }

  private groupItem(n: Extract<Node, { kind: "group" }>): vscode.TreeItem {
    const it = new vscode.TreeItem(n.label, vscode.TreeItemCollapsibleState.Expanded);
    it.id = n.id;
    it.description = n.description;
    if (n.icon) it.iconPath = new vscode.ThemeIcon(n.icon);
    return it;
  }

  private ruleFileItem(rc: RuleChange, file: string): vscode.TreeItem {
    const it = new vscode.TreeItem(path.basename(file));
    it.description = path.dirname(file);
    const root = this.root();
    if (root) it.resourceUri = vscode.Uri.file(path.join(root, file));
    it.iconPath = new vscode.ThemeIcon("law");
    it.command = { command: "lawgate.showRuleFile", title: "Show the change", arguments: [rc, file] };
    return it;
  }

  private whereItem(n: Extract<Node, { kind: "where" }>): vscode.TreeItem {
    const it = new vscode.TreeItem(n.label);
    it.iconPath = new vscode.ThemeIcon("location");
    it.command = open(n.file, n.line);
    return it;
  }

  private fileItem(f: FileReport, governed: number): vscode.TreeItem {
    const it = new vscode.TreeItem(path.basename(f.path), vscode.TreeItemCollapsibleState.Collapsed);
    it.description = fileDescription(f, governed);
    const root = this.root();
    if (root) it.resourceUri = vscode.Uri.file(path.join(root, f.path));
    it.iconPath = f.rule_file || f.lock_file ? new vscode.ThemeIcon("law") : vscode.ThemeIcon.File;
    return it;
  }

  private messageItem(n: Extract<Node, { kind: "message" }>): vscode.TreeItem {
    const it = new vscode.TreeItem(n.label);
    if (n.icon) it.iconPath = new vscode.ThemeIcon(n.icon);
    return it;
  }

  private roots(): Node[] {
    if (this.loading) return [{ kind: "message", label: "Reviewing the branch…", icon: "loading~spin" }];
    if (this.error) return [{ kind: "message", label: this.error, icon: "error" }];
    const r = this.report;
    if (!r) return [];
    const head: Node = {
      kind: "message",
      label: `${r.base} @ ${r.merge_base.slice(0, 8)} → ${r.head.slice(0, 8)} · ${(r.commits ?? []).length} commits`,
      icon: "git-compare",
    };
    return [head, this.lawsGroup(r), ...this.ruleGroup(r), ...this.driftGroup(r), this.changesGroup(r)];
  }

  private lawsGroup(r: Report): Node {
    const laws = r.laws ?? [];
    const broken = laws.filter((l) => l.status === "broken").length;
    const description = `${laws.length}${broken ? ` · ${broken} broken` : ""}`;
    return { kind: "group", id: "laws", label: "Laws touched", description, icon: "law", children: () => laws.map((law): Node => ({ kind: "law", law })) };
  }

  private ruleGroup(r: Report): Node[] {
    const rules = r.rule_changes ?? [];
    if (rules.length === 0) return [];
    const unapproved = rules.filter((x) => !x.approved).length;
    const description = unapproved ? `${unapproved} without approval` : `${rules.length}, all approved`;
    const icon = unapproved ? "error" : "verified";
    return [{ kind: "group", id: "rules", label: "Rule changes", description, icon, children: () => rules.map((rule): Node => ({ kind: "rule", rule })) }];
  }

  private driftGroup(r: Report): Node[] {
    const drift = r.problems ?? [];
    if (drift.length === 0) return [];
    const children = () => drift.map((p): Node => ({ kind: "detail", label: p.message, description: p.machine, icon: "warning", command: open(p.file, p.line ?? 1) }));
    return [{ kind: "group", id: "drift", label: "Declaration drift", description: String(drift.length), icon: "warning", children }];
  }

  private changesGroup(r: Report): Node {
    const files = (r.files ?? [])
      .map((f) => ({ f, hunks: (f.hunks ?? []).filter(governs) }))
      .filter((x) => x.hunks.length > 0 || x.f.rule_file || x.f.lock_file);
    const total = (r.files ?? []).reduce((a, f) => a + (f.hunks?.length ?? 0), 0);
    const governed = files.reduce((a, x) => a + x.hunks.length, 0);
    const children = () => files.map(({ f, hunks }): Node => ({ kind: "file", file: f, hunks }));
    return { kind: "group", id: "files", label: "Changes", description: `${governed} of ${total} governed`, icon: "diff", children };
  }
}
