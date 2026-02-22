import * as vscode from "vscode";

import {
  buildDiagnostics,
  CollectionReferenceLocation,
  SUPPORTED_LANGUAGES,
} from "./diagnostics";
import { MongospectreCodeActionProvider } from "./codeActions";
import { HoverState, MongospectreHoverProvider } from "./hover";
import { runMongospectreCheck, loadConfig } from "./runner";
import { CollectionInfo } from "./types";

const DOCUMENT_SELECTOR: vscode.DocumentSelector = [
  { scheme: "file", language: "go" },
  { scheme: "file", language: "python" },
  { scheme: "file", language: "javascript" },
  { scheme: "file", language: "typescript" },
  { scheme: "file", language: "javascriptreact" },
  { scheme: "file", language: "typescriptreact" },
];

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("mongospectre");
  const diagnostics = vscode.languages.createDiagnosticCollection("mongospectre");
  const statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  statusBar.command = "mongospectre.refresh";
  statusBar.show();

  const hoverState: HoverState = {
    referencesByUri: new Map<string, CollectionReferenceLocation[]>(),
    collectionsByName: new Map<string, CollectionInfo>(),
  };

  let refreshTimer: NodeJS.Timeout | undefined;
  let refreshInFlight: Promise<void> | undefined;

  const refresh = async (): Promise<void> => {
    if (refreshInFlight) {
      return refreshInFlight;
    }

    refreshInFlight = (async () => {
      const folders = vscode.workspace.workspaceFolders;
      if (!folders || folders.length === 0) {
        diagnostics.clear();
        hoverState.referencesByUri = new Map<string, CollectionReferenceLocation[]>();
        hoverState.collectionsByName = new Map<string, CollectionInfo>();
        statusBar.text = "$(circle-slash) mongospectre: no workspace";
        statusBar.tooltip = "Open a workspace to run mongospectre diagnostics.";
        return;
      }

      const cfg = loadConfig();
      statusBar.text = "$(sync~spin) mongospectre: scanning";
      statusBar.tooltip = "Running mongospectre check";

      const diagnosticsByUri = new Map<string, vscode.Diagnostic[]>();
      const referencesByUri = new Map<string, CollectionReferenceLocation[]>();
      const collectionsByName = new Map<string, CollectionInfo>();
      let issueCount = 0;
      let failures = 0;

      const jobs = folders.map(async (folder) => {
        try {
          const report = await runMongospectreCheck(folder, cfg, output);
          const built = await buildDiagnostics(report, folder);
          return { folder, built };
        } catch (err) {
          failures++;
          output.appendLine(`[${folder.name}] check failed: ${String(err)}`);
          return undefined;
        }
      });

      const results = await Promise.all(jobs);
      for (const result of results) {
        if (!result) {
          continue;
        }

        issueCount += result.built.issueCount;
        mergeDiagnostics(diagnosticsByUri, result.built.diagnosticsByUri);
        mergeReferences(referencesByUri, result.built.referencesByUri);
        mergeCollections(collectionsByName, result.built.collectionsByName);
      }

      diagnostics.clear();
      for (const [uriString, entries] of diagnosticsByUri) {
        diagnostics.set(vscode.Uri.parse(uriString), entries);
      }

      hoverState.referencesByUri = referencesByUri;
      hoverState.collectionsByName = collectionsByName;

      const issueLabel = issueCount === 1 ? "issue" : "issues";
      const failureSuffix = failures > 0 ? ` (${failures} folder failed)` : "";
      statusBar.text = `mongospectre: ${issueCount} ${issueLabel}${failureSuffix}`;
      statusBar.tooltip = "Click to refresh mongospectre diagnostics.";
    })().finally(() => {
      refreshInFlight = undefined;
    });

    return refreshInFlight;
  };

  const scheduleRefresh = (): void => {
    const cfg = loadConfig();
    if (!cfg.autoRefresh) {
      return;
    }

    if (refreshTimer) {
      clearTimeout(refreshTimer);
    }
    refreshTimer = setTimeout(() => {
      void refresh();
    }, cfg.debounceMs);
  };

  context.subscriptions.push(output, diagnostics, statusBar);

  context.subscriptions.push(
    vscode.commands.registerCommand("mongospectre.refresh", async () => {
      await refresh();
    }),
  );

  context.subscriptions.push(
    vscode.workspace.onDidSaveTextDocument((document) => {
      if (!SUPPORTED_LANGUAGES.has(document.languageId)) {
        return;
      }
      scheduleRefresh();
    }),
  );

  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("mongospectre")) {
        scheduleRefresh();
      }
    }),
  );

  context.subscriptions.push(
    vscode.workspace.onDidChangeWorkspaceFolders(() => {
      void refresh();
    }),
  );

  context.subscriptions.push(
    vscode.languages.registerHoverProvider(DOCUMENT_SELECTOR, new MongospectreHoverProvider(() => hoverState)),
  );

  context.subscriptions.push(
    vscode.languages.registerCodeActionsProvider(DOCUMENT_SELECTOR, new MongospectreCodeActionProvider(), {
      providedCodeActionKinds: MongospectreCodeActionProvider.providedCodeActionKinds,
    }),
  );

  void refresh();
}

export function deactivate(): void {
  // VS Code disposes registered resources via context subscriptions.
}

function mergeDiagnostics(
  target: Map<string, vscode.Diagnostic[]>,
  source: Map<string, vscode.Diagnostic[]>,
): void {
  for (const [uri, diagnostics] of source) {
    const existing = target.get(uri);
    if (!existing) {
      target.set(uri, [...diagnostics]);
      continue;
    }
    existing.push(...diagnostics);
  }
}

function mergeReferences(
  target: Map<string, CollectionReferenceLocation[]>,
  source: Map<string, CollectionReferenceLocation[]>,
): void {
  for (const [uri, refs] of source) {
    const existing = target.get(uri);
    if (!existing) {
      target.set(uri, [...refs]);
      continue;
    }
    existing.push(...refs);
  }
}

function mergeCollections(
  target: Map<string, CollectionInfo>,
  source: Map<string, CollectionInfo>,
): void {
  for (const [name, info] of source) {
    if (!target.has(name)) {
      target.set(name, info);
    }
  }
}
