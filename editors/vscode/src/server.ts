import * as cp from "child_process";
import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind } from "vscode-languageclient/node";
import { Summary } from "./types";

const GO_RUN_PACKAGE = "github.com/pantacor/lawgate/cmd/lawgate";

export function config(): vscode.WorkspaceConfiguration {
  return vscode.workspace.getConfiguration("lawgate");
}

interface Launch {
  command: string;
  args: string[];
}

// onPath finds an executable named `name` on PATH, the way a shell would.
function onPath(name: string): string | undefined {
  const dirs = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const names = process.platform === "win32" ? [`${name}.exe`, `${name}.cmd`] : [name];
  const candidates = dirs.flatMap((dir) => names.map((n) => path.join(dir, n)));
  return candidates.find((file) => {
    try {
      fs.accessSync(file, fs.constants.X_OK);
      return fs.statSync(file).isFile();
    } catch {
      return false;
    }
  });
}

// launchFor decides how to start the server: a configured binary, `lawgate`
// on PATH, or `go run` of the version the workspace's module pins.
function launchFor(rootArgs: string[]): Launch {
  const configured = config().get<string>("serverPath", "");
  if (configured) return { command: configured, args: ["review", "-lsp", ...rootArgs] };
  const installed = onPath("lawgate");
  if (installed) return { command: installed, args: ["review", "-lsp", ...rootArgs] };
  return { command: "go", args: ["run", GO_RUN_PACKAGE, "review", "-lsp", ...rootArgs] };
}

// serverOptions runs the server from the workspace folder. When lawgate.json
// is deeper, the server is told where it is.
function serverOptions(workspaceDir: string, lawgateDir: string): ServerOptions {
  // The Go extension sets GOROOT in the extension host to the go it found
  // first; when go.mod asks for a newer toolchain, go switches to it and that
  // GOROOT then breaks every compile. go finds its own root without it.
  const env = { ...process.env };
  delete env.GOROOT;
  const rootArgs = lawgateDir === workspaceDir ? [] : ["-root", lawgateDir];
  const launch = launchFor(rootArgs);
  const run = { ...launch, options: { cwd: workspaceDir, env }, transport: TransportKind.stdio };
  return { run, debug: run };
}

export interface ServerEvents {
  onIndexChanged(summary: Summary): void;
}

export class Server {
  private client: LanguageClient | undefined;

  constructor(
    private readonly workspaceDir: string,
    private readonly lawgateDir: string,
    private readonly events: ServerEvents,
  ) {}

  async start(): Promise<void> {
    const options: LanguageClientOptions = {
      documentSelector: [
        { scheme: "file", language: "go", pattern: `${this.lawgateDir}/**/*.go` },
        { scheme: "file", pattern: `${this.lawgateDir}/**/LAWS.md` },
      ],
      initializationOptions: { siteLenses: config().get<boolean>("siteLenses", true) },
      outputChannelName: "Lawgate",
    };
    const client = new LanguageClient("lawgate", "Lawgate", serverOptions(this.workspaceDir, this.lawgateDir), options);
    // The server announces a rebuilt index under its own name; the older
    // review server used another prefix, which is accepted too.
    for (const method of ["lawgate/indexChanged", "lawgateReview/indexChanged"]) {
      client.onNotification(method, (s: Summary) => this.events.onIndexChanged(s));
    }
    this.client = client;
    await client.start();
    this.events.onIndexChanged(await this.exec<Summary>("lawgate.summary"));
  }

  async stop(): Promise<void> {
    await this.client?.stop();
    this.client = undefined;
  }

  async restart(): Promise<void> {
    await this.stop();
    await this.start();
  }

  async exec<T>(command: string, ...args: unknown[]): Promise<T> {
    if (!this.client) throw new Error("Lawgate: the language server is not running");
    return (await this.client.sendRequest("workspace/executeCommand", { command, arguments: args })) as T;
  }
}

// defaultBase is the repository's default branch, else origin/main.
export function defaultBase(dir: string): Promise<string> {
  return new Promise((resolve) => {
    cp.execFile("git", ["symbolic-ref", "--short", "refs/remotes/origin/HEAD"], { cwd: dir }, (err, stdout) => {
      const ref = stdout.trim();
      resolve(err || !ref ? "origin/main" : ref);
    });
  });
}
