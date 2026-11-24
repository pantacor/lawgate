import * as vscode from "vscode";

const STYLE = `
body{font-family:var(--vscode-font-family);font-size:var(--vscode-font-size);color:var(--vscode-foreground);padding:0 16px 32px;line-height:1.5}
table{border-collapse:collapse;width:100%}td,th{border:1px solid var(--vscode-panel-border);padding:4px 8px;text-align:left;vertical-align:top}
code{font-family:var(--vscode-editor-font-family);background:var(--vscode-textCodeBlock-background);padding:0 3px;border-radius:3px}
details{margin:6px 0}summary{cursor:pointer;font-weight:600}blockquote{margin:4px 0;padding-left:10px;border-left:3px solid var(--vscode-textBlockQuote-border);color:var(--vscode-descriptionForeground)}
a{color:var(--vscode-textLink-foreground)}`;

let panel: vscode.WebviewPanel | undefined;

function page(webview: vscode.Webview, body: string): string {
  const csp = `default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; img-src ${webview.cspSource} https:;`;
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><style>${STYLE}</style></head><body>${body}</body></html>`;
}

function reportPanel(title: string): vscode.WebviewPanel {
  if (panel) return panel;
  panel = vscode.window.createWebviewPanel("lawgate.report", title, vscode.ViewColumn.Beside, { enableScripts: false, enableFindWidget: true });
  panel.onDidDispose(() => (panel = undefined));
  return panel;
}

// showMarkdown renders Markdown in the report panel. Scripts are off and the
// content security policy allows no remote code.
export async function showMarkdown(title: string, markdown: string): Promise<void> {
  const html = (await vscode.commands.executeCommand<string>("markdown.api.render", markdown)) ?? "";
  const shown = reportPanel(title);
  shown.title = title;
  shown.webview.html = page(shown.webview, html);
  shown.reveal(vscode.ViewColumn.Beside, true);
}
