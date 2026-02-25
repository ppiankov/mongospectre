import * as vscode from "vscode";

interface FindingDiagnosticData {
  type: string;
  database?: string;
  collection?: string;
  index?: string;
}

export class MongospectreCodeActionProvider implements vscode.CodeActionProvider {
  public static readonly providedCodeActionKinds = [vscode.CodeActionKind.QuickFix];

  async provideCodeActions(
    document: vscode.TextDocument,
    _range: vscode.Range,
    context: vscode.CodeActionContext,
  ): Promise<vscode.CodeAction[]> {
    const folder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!folder) {
      return [];
    }

    const actions: vscode.CodeAction[] = [];
    const used = new Set<string>();

    for (const diagnostic of context.diagnostics) {
      if (diagnostic.source !== "mongospectre") {
        continue;
      }

      const data = (diagnostic as vscode.Diagnostic & { data?: unknown }).data as FindingDiagnosticData | undefined;
      const findingType = typeof diagnostic.code === "string" ? diagnostic.code : data?.type;
      if (!findingType || !data?.collection) {
        continue;
      }

      const scope = buildScope(data);
      const ignoreLine = `${findingType} ${scope}`;
      const dedupKey = `${findingType}:${scope}`;
      if (used.has(dedupKey)) {
        continue;
      }
      used.add(dedupKey);

      const action = await buildIgnoreRuleAction(folder, diagnostic, ignoreLine);
      if (action) {
        actions.push(action);
      }
    }

    return actions;
  }
}

function buildScope(data: FindingDiagnosticData): string {
  const collection = data.collection ?? "";
  if (data.database && data.index) {
    return `${data.database}.${collection}.${data.index}`;
  }
  if (data.database) {
    return `${data.database}.${collection}`;
  }
  return collection;
}

async function buildIgnoreRuleAction(
  folder: vscode.WorkspaceFolder,
  diagnostic: vscode.Diagnostic,
  ignoreLine: string,
): Promise<vscode.CodeAction | undefined> {
  const ignoreUri = vscode.Uri.joinPath(folder.uri, ".mongospectreignore");

  let existing: vscode.TextDocument | undefined;
  try {
    existing = await vscode.workspace.openTextDocument(ignoreUri);
  } catch {
    existing = undefined;
  }

  if (existing && hasIgnoreRule(existing.getText(), ignoreLine)) {
    return undefined;
  }

  const action = new vscode.CodeAction(`mongospectre: Ignore \`${ignoreLine}\``, vscode.CodeActionKind.QuickFix);
  action.diagnostics = [diagnostic];

  const edit = new vscode.WorkspaceEdit();
  if (!existing) {
    edit.createFile(ignoreUri, { ignoreIfExists: true });
    edit.insert(ignoreUri, new vscode.Position(0, 0), `${ignoreLine}\n`);
  } else {
    const fullText = existing.getText();
    const end = existing.lineAt(existing.lineCount - 1).range.end;
    const prefix = fullText.endsWith("\n") || fullText.length === 0 ? "" : "\n";
    edit.insert(ignoreUri, end, `${prefix}${ignoreLine}\n`);
  }

  action.edit = edit;
  return action;
}

function hasIgnoreRule(content: string, ignoreLine: string): boolean {
  const target = ignoreLine.trim();
  return content
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== "" && !line.startsWith("#"))
    .includes(target);
}
