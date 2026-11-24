import * as path from "path";
import * as vscode from "vscode";
import { CodeRef, Commit, HunkReport, Report, lawStatusIcon, governs, refLabel } from "./types";

function codes(names: string[]): string {
  return names.map((n) => `\`${n}\``).join(", ");
}

function siteLines(h: HunkReport): string[] {
  const sites = (h.sites ?? []).map((s) => `**Site \`${s.tag ?? s.step}\`** · ${s.kind} · \`${s.state}/${s.step}\`${s.effect ? ` — ${s.effect}` : ""}${s.caught ? ` (caught: ${s.caught})` : ""}`);
  const declarations = (h.declarations ?? []).map((s) => `**Declaration of \`${s.tag ?? s.step}\`** (\`${s.state}/${s.step}\`, resolved at ${s.file}:${s.line})`);
  return [...sites, ...declarations];
}

function refLine(title: string, refs: CodeRef[] | undefined): string[] {
  if (!refs?.length) return [];
  const parts = refs.map((r) => `\`${refLabel(r)}\`${r.law ? ` (law \`${r.law}\`)` : ""}`);
  return [`**${title}:** ${parts.join(", ")}`];
}

function lawLines(h: HunkReport, r: Report): string[] {
  const out: string[] = [];
  if (h.laws?.length) {
    const marks = h.laws.map((n) => `${lawStatusIcon(r.laws?.find((l) => l.name === n))} \`${n}\``);
    out.push(`**Laws:** ${marks.join(" ")}`);
  }
  if (h.law_text?.length) out.push(`**Law text changed:** ${codes(h.law_text)}`);
  if (h.removed_tags?.length) out.push(`**Removed log tags:** ${codes(h.removed_tags)}`);
  return out;
}

function whyLines(h: HunkReport, commits: Map<string, Commit>): string[] {
  const out = (h.commits ?? []).map((s) => `**Why:** \`${s}\` ${commits.get(s)?.subject ?? ""}`);
  if (h.uncommitted) out.push("**Why:** not committed yet");
  return out;
}

function hunkBody(h: HunkReport, r: Report, commits: Map<string, Commit>): string {
  const states = h.states?.length ? [`**States:** ${codes(h.states)}`] : [];
  const lines = [
    ...states,
    ...siteLines(h),
    ...refLine("Protected subjects", h.subjects),
    ...refLine("Proofs", h.tests),
    ...lawLines(h, r),
    ...whyLines(h, commits),
  ];
  return lines.join("\n\n");
}

function threadLabel(h: HunkReport): string {
  if (h.laws?.length) return `Laws: ${h.laws.join(", ")}`;
  return h.states?.join(", ") || "Lawgate";
}

// Notes puts a read-only comment thread on every governed hunk, so the
// states, laws and reasons show next to the code in editors and diff views.
export class Notes implements vscode.Disposable {
  private readonly controller = vscode.comments.createCommentController("lawgate", "Lawgate");
  private threads: vscode.CommentThread[] = [];

  constructor() {
    this.controller.options = { prompt: "Lawgate notes are read-only", placeHolder: "" };
  }

  clear(): void {
    for (const t of this.threads) t.dispose();
    this.threads = [];
  }

  show(root: string, r: Report): void {
    this.clear();
    const commits = new Map((r.commits ?? []).map((c) => [c.short, c] as [string, Commit]));
    for (const f of r.files ?? []) {
      if (f.status === "deleted") continue;
      const uri = vscode.Uri.file(path.join(root, f.path));
      for (const h of (f.hunks ?? []).filter(governs)) this.addThread(uri, h, hunkBody(h, r, commits));
    }
  }

  private addThread(uri: vscode.Uri, h: HunkReport, body: string): void {
    const start = Math.max(0, h.new_start - 1);
    const end = Math.max(start, h.new_start + Math.max(h.new_lines, 1) - 2);
    const comment: vscode.Comment = { body: new vscode.MarkdownString(body), mode: vscode.CommentMode.Preview, author: { name: "Lawgate" } };
    const thread = this.controller.createCommentThread(uri, new vscode.Range(start, 0, end, 0), [comment]);
    thread.canReply = false;
    thread.label = threadLabel(h);
    thread.collapsibleState = vscode.CommentThreadCollapsibleState.Collapsed;
    this.threads.push(thread);
  }

  dispose(): void {
    this.clear();
    this.controller.dispose();
  }
}
