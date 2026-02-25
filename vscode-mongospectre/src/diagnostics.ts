import * as path from "path";
import * as vscode from "vscode";

import { CheckReport, CollectionInfo, CollectionRef, Finding, FindingSeverity } from "./types";

export const SUPPORTED_LANGUAGES = new Set<string>([
  "go",
  "python",
  "javascript",
  "typescript",
  "javascriptreact",
  "typescriptreact",
]);

export interface CollectionReferenceLocation {
  uri: vscode.Uri;
  range: vscode.Range;
  collection: string;
}

export interface DiagnosticsBuildResult {
  diagnosticsByUri: Map<string, vscode.Diagnostic[]>;
  referencesByUri: Map<string, CollectionReferenceLocation[]>;
  collectionsByName: Map<string, CollectionInfo>;
  issueCount: number;
}

interface FindingDiagnosticData {
  type: string;
  database?: string;
  collection?: string;
  index?: string;
}

export async function buildDiagnostics(
  report: CheckReport,
  folder: vscode.WorkspaceFolder,
): Promise<DiagnosticsBuildResult> {
  const diagnosticsByUri = new Map<string, vscode.Diagnostic[]>();
  const referencesByUri = new Map<string, CollectionReferenceLocation[]>();
  const collectionsByName = buildCollectionMap(report.collections ?? []);
  const issueFindingsByCollection = groupIssueFindings(report.findings);

  const refs = report.scan?.refs ?? [];
  const docCache = new Map<string, vscode.TextDocument>();
  const diagDedup = new Set<string>();

  for (const ref of refs) {
    const uri = resolveRefURI(folder, ref);
    const document = await openDocumentCached(uri, docCache);
    if (!document || !SUPPORTED_LANGUAGES.has(document.languageId)) {
      continue;
    }

    const range = findCollectionRange(document, ref.line, ref.collection);
    if (!range) {
      continue;
    }

    const refKey = uri.toString();
    pushReference(referencesByUri, refKey, {
      uri,
      range,
      collection: ref.collection,
    });

    const findings = issueFindingsByCollection.get(ref.collection.toLowerCase()) ?? [];
    for (const finding of findings) {
      const diag = new vscode.Diagnostic(range, finding.message, severityToDiagnostic(finding.severity));
      diag.source = "mongospectre";
      diag.code = finding.type;
      // Diagnostic.data is available since VS Code 1.73 but some @types/vscode
      // versions omit it from the type definition. Cast to attach metadata.
      (diag as vscode.Diagnostic & { data?: unknown }).data = {
        type: finding.type,
        database: finding.database,
        collection: finding.collection,
        index: finding.index,
      } satisfies FindingDiagnosticData;

      const signature = `${refKey}:${range.start.line}:${range.start.character}:${finding.type}:${finding.message}`;
      if (diagDedup.has(signature)) {
        continue;
      }
      diagDedup.add(signature);

      const bucket = diagnosticsByUri.get(refKey);
      if (!bucket) {
        diagnosticsByUri.set(refKey, [diag]);
      } else {
        bucket.push(diag);
      }
    }
  }

  return {
    diagnosticsByUri,
    referencesByUri,
    collectionsByName,
    issueCount: countIssueFindings(report.findings),
  };
}

export function countIssueFindings(findings: Finding[]): number {
  let total = 0;
  for (const finding of findings) {
    if (isIssueFinding(finding)) {
      total++;
    }
  }
  return total;
}

function groupIssueFindings(findings: Finding[]): Map<string, Finding[]> {
  const out = new Map<string, Finding[]>();
  for (const finding of findings) {
    if (!isIssueFinding(finding) || !finding.collection) {
      continue;
    }
    const key = finding.collection.toLowerCase();
    const bucket = out.get(key);
    if (!bucket) {
      out.set(key, [finding]);
    } else {
      bucket.push(finding);
    }
  }
  return out;
}

function isIssueFinding(finding: Finding): boolean {
  return finding.type !== "OK";
}

function buildCollectionMap(collections: CollectionInfo[]): Map<string, CollectionInfo> {
  const byName = new Map<string, CollectionInfo>();
  for (const collection of collections) {
    const key = collection.name.toLowerCase();
    if (!byName.has(key)) {
      byName.set(key, collection);
    }
  }
  return byName;
}

async function openDocumentCached(
  uri: vscode.Uri,
  cache: Map<string, vscode.TextDocument>,
): Promise<vscode.TextDocument | undefined> {
  const key = uri.toString();
  const cached = cache.get(key);
  if (cached) {
    return cached;
  }
  try {
    const document = await vscode.workspace.openTextDocument(uri);
    cache.set(key, document);
    return document;
  } catch {
    return undefined;
  }
}

function resolveRefURI(folder: vscode.WorkspaceFolder, ref: CollectionRef): vscode.Uri {
  if (path.isAbsolute(ref.file)) {
    return vscode.Uri.file(ref.file);
  }
  return vscode.Uri.file(path.join(folder.uri.fsPath, ref.file));
}

function findCollectionRange(
  document: vscode.TextDocument,
  oneBasedLine: number,
  collection: string,
): vscode.Range | undefined {
  const lineIndex = oneBasedLine - 1;
  if (lineIndex < 0 || lineIndex >= document.lineCount) {
    return undefined;
  }

  const line = document.lineAt(lineIndex).text;
  if (line.length === 0) {
    return new vscode.Range(lineIndex, 0, lineIndex, 0);
  }

  const start = indexOfCaseInsensitive(line, collection);
  if (start >= 0) {
    return new vscode.Range(lineIndex, start, lineIndex, start + collection.length);
  }

  return new vscode.Range(lineIndex, 0, lineIndex, line.length);
}

function indexOfCaseInsensitive(haystack: string, needle: string): number {
  return haystack.toLowerCase().indexOf(needle.toLowerCase());
}

function severityToDiagnostic(severity: FindingSeverity): vscode.DiagnosticSeverity {
  switch (severity) {
    case "high":
      return vscode.DiagnosticSeverity.Error;
    case "medium":
      return vscode.DiagnosticSeverity.Warning;
    case "low":
      return vscode.DiagnosticSeverity.Information;
    default:
      return vscode.DiagnosticSeverity.Hint;
  }
}

function pushReference(
  referencesByUri: Map<string, CollectionReferenceLocation[]>,
  key: string,
  ref: CollectionReferenceLocation,
): void {
  const bucket = referencesByUri.get(key);
  if (!bucket) {
    referencesByUri.set(key, [ref]);
    return;
  }
  bucket.push(ref);
}
