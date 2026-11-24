// Shapes of the language server's answers (lawgate review -lsp). Fields the
// server may leave out are optional: the subjects and tests of a law are
// resolved by go/parser, so a repository in another language has none.

export interface Verdict {
  machine: string;
  status: "holds" | "broken" | "pending";
  pending?: string;
  violations?: string[];
}

// CodeRef points at a function or a range of lines: the subject a law
// protects, or a test that proves a law.
export interface CodeRef {
  name?: string;
  law?: string;
  file: string;
  line: number;
  end_line?: number;
}

export interface Site {
  machine: string;
  state: string;
  status?: string;
  step: string;
  step_doc?: string;
  tag?: string;
  kind: string;
  effect?: string;
  file: string;
  line: number;
  laws?: string[];
  caught?: string;
  decl_file?: string;
  decl_line?: number;
}

export interface State {
  machine: string;
  id: string;
  func?: string;
  status?: string;
  doc?: string;
  when?: string;
  woken_by?: string;
  terminal?: boolean;
  wait?: boolean;
  edges?: string[];
  laws?: string[];
  file?: string;
  start_line?: number;
  end_line?: number;
}

export interface Law {
  name: string;
  title?: string;
  parent?: string;
  file?: string;
  line?: number;
  end_line?: number;
  text?: string;
  code_doc?: string;
  subject?: string;
  subjects?: CodeRef[];
  tests?: CodeRef[];
  verdicts?: Verdict[];
}

export interface Problem {
  machine: string;
  file: string;
  line?: number;
  message: string;
}

export interface Index {
  root: string;
  sites: Site[];
  states: State[];
  laws: Record<string, Law>;
  problems: Problem[] | null;
}

export interface Commit {
  sha: string;
  short: string;
  author: string;
  date: string;
  subject: string;
  body?: string;
  trailers?: Record<string, string[]>;
  uncommitted?: boolean;
  merge_request?: string;
}

export interface HunkReport {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  header?: string;
  added?: string[];
  removed?: string[];
  sites?: Site[];
  declarations?: Site[];
  subjects?: CodeRef[];
  tests?: CodeRef[];
  states?: string[];
  laws?: string[];
  law_text?: string[];
  removed_tags?: string[];
  commits?: string[];
  uncommitted?: boolean;
}

export interface FileReport {
  path: string;
  old_path?: string;
  status: string;
  rule_file?: boolean;
  lock_file?: boolean;
  hunks: HunkReport[] | null;
}

export interface LawImpact {
  name: string;
  title?: string;
  parent?: string;
  file?: string;
  line?: number;
  status: string;
  verdicts?: Verdict[];
  text_changed?: boolean;
  where: string[];
  known: boolean;
  subjects?: CodeRef[];
  tests?: CodeRef[];
}

export interface RuleChange {
  commit: string;
  subject: string;
  files: string[];
  approved_by?: string[];
  approved: boolean;
}

export interface Report {
  base: string;
  merge_base: string;
  head: string;
  files: FileReport[] | null;
  laws: LawImpact[] | null;
  commits: Commit[] | null;
  rule_changes: RuleChange[] | null;
  problems: Problem[] | null;
}

export interface Summary {
  root: string;
  sites: number;
  states: number;
  laws: number;
  holds: number;
  broken: number;
  pending: number;
  drift: number;
}

export function governs(h: HunkReport): boolean {
  const touched = [h.sites, h.declarations, h.subjects, h.tests, h.states, h.laws, h.law_text, h.removed_tags];
  return touched.some((list) => (list?.length ?? 0) > 0);
}

export function statusIcon(status: string): string {
  switch (status) {
    case "holds":
      return "✅";
    case "broken":
      return "❌";
    case "pending":
      return "⏳";
    case "unknown":
      return "❓";
  }
  return "➖";
}

export function lawStatusIcon(l: LawImpact | undefined): string {
  return statusIcon(l?.known ? l.status : "unknown");
}

export function refLabel(r: CodeRef): string {
  return r.name ?? `${r.file}:${r.line}`;
}
