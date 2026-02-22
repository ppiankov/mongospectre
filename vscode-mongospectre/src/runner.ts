import { execFile, ExecFileException } from "child_process";
import * as vscode from "vscode";

import { CheckReport, MongospectreConfig } from "./types";

interface CommandResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export function loadConfig(): MongospectreConfig {
  const cfg = vscode.workspace.getConfiguration("mongospectre");
  return {
    uri: cfg.get<string>("uri", "mongodb://localhost:27017"),
    database: cfg.get<string>("database", ""),
    autoRefresh: cfg.get<boolean>("autoRefresh", true),
    binaryPath: cfg.get<string>("binaryPath", "mongospectre"),
    debounceMs: cfg.get<number>("debounceMs", 800),
  };
}

export async function runMongospectreCheck(
  folder: vscode.WorkspaceFolder,
  config: MongospectreConfig,
  output: vscode.OutputChannel,
): Promise<CheckReport> {
  const args = ["check", "--format", "json", "--repo", folder.uri.fsPath, "--uri", config.uri, "--no-interactive"];
  if (config.database.trim() !== "") {
    args.push("--database", config.database.trim());
  }

  output.appendLine(`[${folder.name}] ${config.binaryPath} ${args.join(" ")}`);
  const result = await runCommand(config.binaryPath, args, folder.uri.fsPath);

  if (result.stderr.trim() !== "") {
    output.appendLine(`[${folder.name}] stderr:\n${result.stderr.trim()}`);
  }

  if (result.stdout.trim() === "") {
    throw new Error(`mongospectre produced no JSON output for workspace ${folder.name}`);
  }

  let report: CheckReport;
  try {
    report = JSON.parse(result.stdout) as CheckReport;
  } catch (err) {
    throw new Error(`invalid JSON from mongospectre in ${folder.name}: ${String(err)}`);
  }

  if (!Array.isArray(report.findings) || !report.summary) {
    throw new Error(`unexpected mongospectre report schema in ${folder.name}`);
  }

  return report;
}

function runCommand(binary: string, args: string[], cwd: string): Promise<CommandResult> {
  return new Promise((resolve, reject) => {
    execFile(
      binary,
      args,
      {
        cwd,
        maxBuffer: 20 * 1024 * 1024,
        env: process.env,
      },
      (error, stdout, stderr) => {
        const execErr = error as ExecFileException | null;
        const code = normalizeExitCode(execErr?.code);

        if (execErr && isBinaryMissing(execErr)) {
          reject(new Error(`mongospectre binary not found: ${binary}`));
          return;
        }

        if (execErr && stdout.trim() === "") {
          const detail = stderr.trim() || execErr.message;
          reject(new Error(detail));
          return;
        }

        resolve({
          stdout,
          stderr,
          exitCode: code,
        });
      },
    );
  });
}

function normalizeExitCode(code: string | number | null | undefined): number {
  if (typeof code === "number") {
    return code;
  }
  return 0;
}

function isBinaryMissing(err: ExecFileException): boolean {
  return err.code === "ENOENT" || /ENOENT/.test(err.message);
}
