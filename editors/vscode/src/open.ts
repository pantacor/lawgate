import * as path from "path";
import * as vscode from "vscode";
import { Commit, RuleChange } from "./types";

// Opening files and diffs. Every path the server sends is relative to the
// directory that holds lawgate.json.

interface GitAPI {
  toGitUri(uri: vscode.Uri, ref: string): vscode.Uri;
}

function gitAPI(): GitAPI | undefined {
  const ext = vscode.extensions.getExtension("vscode.git");
  return ext?.isActive ? (ext.exports.getAPI(1) as GitAPI) : undefined;
}

function lineRange(line: number): vscode.Range {
  const pos = new vscode.Position(Math.max(0, line - 1), 0);
  return new vscode.Range(pos, pos);
}

export function fileUri(root: string, file: string): vscode.Uri {
  return vscode.Uri.file(path.isAbsolute(file) ? file : path.join(root, file));
}

export async function openLocation(root: string, file: string, line: number): Promise<void> {
  await vscode.window.showTextDocument(fileUri(root, file), { selection: lineRange(line), preview: true });
}

// openHunk shows a changed hunk against the merge base, or just the file when
// the Git extension is not there.
export async function openHunk(root: string, mergeBase: string | undefined, file: string, line: number): Promise<void> {
  const git = gitAPI();
  if (!git || !mergeBase) return openLocation(root, file, line);
  const uri = fileUri(root, file);
  const title = `${path.basename(file)} (${mergeBase.slice(0, 8)} ↔ working tree)`;
  await vscode.commands.executeCommand("vscode.diff", git.toGitUri(uri, mergeBase), uri, title, { selection: lineRange(line), preview: true });
}

type RuleDiff = [vscode.Uri, vscode.Uri, vscode.Uri];

// A rule change is shown as the commit's own diff of its rule files
// (commit~1 to commit), not against the branch base: it is what that commit
// changed in the rules, which is what its approval covers.
function ruleDiff(root: string, rc: RuleChange, file: string): RuleDiff | undefined {
  const git = gitAPI();
  if (!git) return undefined;
  const uri = fileUri(root, file);
  return [uri, git.toGitUri(uri, `${rc.commit}~1`), git.toGitUri(uri, rc.commit)];
}

export async function showRuleFile(root: string, rc: RuleChange, file: string): Promise<void> {
  const diff = ruleDiff(root, rc, file);
  if (!diff) return openLocation(root, file, 1);
  const title = `${path.basename(file)} (${rc.commit}~1 ↔ ${rc.commit})`;
  await vscode.commands.executeCommand("vscode.diff", diff[1], diff[2], title, { preview: true });
}

export async function showRuleChange(root: string, rc: RuleChange): Promise<void> {
  const diffs = rc.files.map((f) => ruleDiff(root, rc, f)).filter((d): d is RuleDiff => d !== undefined);
  if (diffs.length === 0) return;
  if (diffs.length === 1) return showRuleFile(root, rc, rc.files[0]);
  await vscode.commands.executeCommand("vscode.changes", `Rule change ${rc.commit} ${rc.subject}`, diffs);
}

export function commitMarkdown(c: Commit): string {
  const lines = [`**${c.short}** ${c.subject}`, `${c.author}, ${c.date}`];
  if (c.body) lines.push(c.body.split("\n").map((l) => `> ${l}`).join("\n"));
  for (const [k, vs] of Object.entries(c.trailers ?? {})) lines.push(`${k}: ${vs.join(", ")}`);
  if (c.merge_request) lines.push(`[Merge request](${c.merge_request})`);
  return lines.join("\n\n");
}
