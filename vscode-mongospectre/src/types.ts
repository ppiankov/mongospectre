export type FindingSeverity = "high" | "medium" | "low" | "info";

export interface ReportMetadata {
  version?: string;
  command?: string;
  timestamp?: string;
  database?: string;
  mongodbVersion?: string;
  repoPath?: string;
  uriHash?: string;
}

export interface Finding {
  type: string;
  severity: FindingSeverity;
  database?: string;
  collection?: string;
  index?: string;
  message: string;
}

export interface ReportSummary {
  total: number;
  high: number;
  medium: number;
  low: number;
  info: number;
}

export interface CollectionRef {
  collection: string;
  file: string;
  line: number;
  pattern: string;
}

export interface ScanResult {
  repoPath: string;
  refs: CollectionRef[];
  collections: string[];
  filesScanned: number;
  filesSkipped?: number;
}

export interface IndexStats {
  ops: number;
  since: string;
}

export interface KeyField {
  field: string;
  direction: number;
}

export interface IndexInfo {
  name: string;
  key: KeyField[];
  unique?: boolean;
  sparse?: boolean;
  ttl?: number;
  stats?: IndexStats;
}

export interface CollectionInfo {
  name: string;
  database: string;
  type: string;
  docCount: number;
  size?: number;
  avgObjSize?: number;
  storageSize?: number;
  indexes: IndexInfo[];
}

export interface CheckReport {
  metadata: ReportMetadata;
  findings: Finding[];
  maxSeverity: FindingSeverity;
  summary: ReportSummary;
  scan?: ScanResult;
  collections?: CollectionInfo[];
}

export interface MongospectreConfig {
  uri: string;
  database: string;
  autoRefresh: boolean;
  binaryPath: string;
  debounceMs: number;
}
