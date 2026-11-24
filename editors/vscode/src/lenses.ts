import * as path from "path";
import * as vscode from "vscode";
import { openLocation } from "./open";
import { CodeRef, Index } from "./types";

// The code lenses the server draws run these client commands, with the
// arguments it names. A lens whose target is not in the index does nothing.

type QuickItem = vscode.QuickPickItem & { file?: string; line?: number };

export async function showLaw(root: string, index: Index, name: string): Promise<void> {
  const law = index.laws[name];
  if (law?.file && law.line) await openLocation(root, law.file, law.line);
}

// From the code, go to the declaration; from the declaration, to the code.
export async function showSite(root: string, index: Index, machine: string, state: string, step: string, name: string): Promise<void> {
  const site = index.sites.find((x) => x.machine === machine && x.state === state && x.step === step && (x.tag ?? x.step) === name);
  if (!site) return;
  const editor = vscode.window.activeTextEditor;
  const here = editor ? path.relative(root, editor.document.uri.fsPath) : "";
  if (site.decl_file && here !== site.decl_file) await openLocation(root, site.decl_file, site.decl_line ?? 1);
  else await openLocation(root, site.file, site.line);
}

function stateItems(index: Index, machine: string, id: string): QuickItem[] {
  const state = index.states.find((x) => x.machine === machine && x.id === id);
  if (!state) return [];
  const detail = [state.doc, state.when && `When: ${state.when}`, state.woken_by && `Woken by: ${state.woken_by}`].filter(Boolean).join(" · ");
  const head: QuickItem = { label: `$(symbol-enum) ${machine}/${id}`, detail };
  const edges = (state.edges ?? []).map((e): QuickItem => ({ label: `$(arrow-right) ${e}` }));
  const sites = index.sites
    .filter((x) => x.machine === machine && x.state === id)
    .map((s): QuickItem => ({ label: `$(symbol-event) ${s.tag ?? s.step} · ${s.kind}`, description: `${s.step} · line ${s.line}`, detail: s.effect, file: s.file, line: s.line }));
  const laws = (state.laws ?? []).map((n): QuickItem => {
    const law = index.laws[n];
    return { label: `$(law) ${n}`, description: law ? "law" : "not written in a LAWS.md", file: law?.file, line: law?.line };
  });
  return [head, ...edges, ...sites, ...laws];
}

export async function showState(root: string, index: Index, machine: string, id: string): Promise<void> {
  const items = stateItems(index, machine, id);
  if (items.length === 0) return;
  const pick = await vscode.window.showQuickPick(items, { title: `State ${machine}/${id}`, matchOnDetail: true });
  if (pick?.file && pick.line) await openLocation(root, pick.file, pick.line);
}

function refItem(r: CodeRef, role: string): QuickItem {
  return { label: `$(${role === "subject" ? "shield" : "beaker"}) ${r.name ?? r.file}`, description: `${role} · ${r.file}:${r.line}`, file: r.file, line: r.line };
}

// showLawCode lists what a law protects and what proves it, so a subject's
// lens ("protected by law X") and a test's lens ("proves law X") both lead to
// the other side. With one place to go it goes there.
export async function showLawCode(root: string, index: Index, name: string, role: "subject" | "test"): Promise<void> {
  const law = index.laws[name];
  if (!law) return;
  const refs = role === "subject" ? (law.tests ?? []) : (law.subjects ?? []);
  const items: QuickItem[] = [
    { label: `$(law) ${name}`, description: "law", file: law.file, line: law.line },
    ...refs.map((r) => refItem(r, role === "subject" ? "test" : "subject")),
  ];
  const pick = items.length === 2 ? items[1] : await vscode.window.showQuickPick(items, { title: `Law ${name}` });
  if (pick?.file && pick.line) await openLocation(root, pick.file, pick.line);
}
