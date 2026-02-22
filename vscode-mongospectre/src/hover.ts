import * as vscode from "vscode";

import { CollectionInfo, IndexInfo } from "./types";
import { CollectionReferenceLocation } from "./diagnostics";

export interface HoverState {
  referencesByUri: Map<string, CollectionReferenceLocation[]>;
  collectionsByName: Map<string, CollectionInfo>;
}

export class MongospectreHoverProvider implements vscode.HoverProvider {
  constructor(private readonly getState: () => HoverState) {}

  provideHover(document: vscode.TextDocument, position: vscode.Position): vscode.Hover | undefined {
    const state = this.getState();
    const refs = state.referencesByUri.get(document.uri.toString());
    if (!refs || refs.length === 0) {
      return undefined;
    }

    const ref = refs.find((candidate) => candidate.range.contains(position));
    if (!ref) {
      return undefined;
    }

    const metadata = state.collectionsByName.get(ref.collection.toLowerCase());
    const markdown = new vscode.MarkdownString();
    markdown.appendMarkdown("**mongospectre**\n\n");
    markdown.appendMarkdown(`Collection: \`${ref.collection}\`\n\n`);

    if (!metadata) {
      markdown.appendMarkdown("Collection metadata unavailable from the latest check run.");
      return new vscode.Hover(markdown, ref.range);
    }

    markdown.appendMarkdown(`Database: \`${metadata.database}\`\n\n`);
    markdown.appendMarkdown(`Document count: **${metadata.docCount.toLocaleString()}**\n\n`);

    if (metadata.indexes.length > 0) {
      markdown.appendMarkdown("Indexes:\n");
      for (const index of metadata.indexes) {
        markdown.appendMarkdown(`- ${formatIndex(index)}\n`);
      }
      markdown.appendMarkdown("\n");
    } else {
      markdown.appendMarkdown("Indexes: none\n\n");
    }

    markdown.appendMarkdown(`Last query time: ${describeLastQuery(metadata)}\n`);

    return new vscode.Hover(markdown, ref.range);
  }
}

function formatIndex(index: IndexInfo): string {
  const key = index.key.map((part) => `${part.field}:${part.direction}`).join(", ");
  const segments = [`\`${index.name}\``, `\`{${key}}\``];
  if (index.stats) {
    segments.push(`ops=${index.stats.ops.toLocaleString()}`);
    if (index.stats.since) {
      segments.push(`since=${toDateLabel(index.stats.since)}`);
    }
  }
  return segments.join(" | ");
}

function describeLastQuery(collection: CollectionInfo): string {
  const sinceValues: number[] = [];
  for (const index of collection.indexes) {
    const since = index.stats?.since;
    if (!since) {
      continue;
    }
    const parsed = Date.parse(since);
    if (Number.isNaN(parsed)) {
      continue;
    }
    sinceValues.push(parsed);
  }

  if (sinceValues.length === 0) {
    return "unknown";
  }

  const latestSince = new Date(Math.max(...sinceValues));
  return `>= ${latestSince.toISOString()} (exact query timestamp not exposed by $indexStats)`;
}

function toDateLabel(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  return new Date(parsed).toISOString();
}
