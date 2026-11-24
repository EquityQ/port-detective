export type Protocol = "tcp" | "udp";
export type ScanPreset = "web" | "python" | "database" | "project";
export type WaitState = "open" | "free";

export interface ProjectInfo {
  name?: string;
  framework?: string;
  path?: string;
  version?: string;
}

export interface PortDiagnostic {
  port: number;
  is_open: boolean;
  protocol: string;
  pid: number;
  process_name: string;
  cmdline: string;
  memory_mb: number;
  cpu_percent: number;
  status: string;
  project?: ProjectInfo;
  bind_addresses?: string[];
  listener_table_missing?: boolean;
  post_action_status?: string;
}

export interface KillResult {
  port: number;
  dry_run: boolean;
  message: string;
  pre_action_diagnostic?: PortDiagnostic | null;
  /** @deprecated Alias for `pre_action_diagnostic` retained for backward compatibility. */
  diagnostic?: PortDiagnostic | null;
  post_action_status?: string;
  preview_id?: string;
  preview_expires_at_unix?: number;
}

export interface FreePortResult {
  port: number;
  protocol: string;
  ipv4: boolean;
  ipv6: boolean;
  reserved: boolean;
  message: string;
}

export interface DoctorReport {
  version: string;
  os: string;
  arch: string;
  binary_path: string;
  cwd: string;
  project_path?: string;
  status: string;
  self_check?: PortDiagnostic | null;
  metadata_accessible: boolean;
  hints: string[];
}

export interface ClientOptions {
  /** Overrides PORT_DETECTIVE_BIN, the bundled platform binary, and PATH, in that order. */
  binaryPath?: string;
  /** Working directory passed to the CLI. */
  cwd?: string;
  /** Process timeout in milliseconds. Use null to disable it. Defaults to 30000. */
  timeout?: number | null;
  /** Maximum captured bytes per output stream. Defaults to 10 MiB. */
  maxBuffer?: number;
}

export interface AsyncClientOptions extends ClientOptions {
  /** Aborts the child CLI process and rejects with AbortError. */
  signal?: AbortSignal;
}

export interface InspectOptions extends ClientOptions {
  /** Transport protocol to inspect. Defaults to "tcp". */
  protocol?: Protocol;
}

export interface AsyncInspectOptions extends AsyncClientOptions {
  /** Transport protocol to inspect. Defaults to "tcp". */
  protocol?: Protocol;
}

export interface ScanOptions extends ClientOptions {
  /** Transport protocol to scan. Defaults to "tcp". */
  protocol?: Protocol;
  /** Explicit port preset; cannot be combined with an explicit ports list. */
  preset?: ScanPreset;
  /** Project directory used with `preset: "project"`. */
  projectPath?: string;
}

export interface AsyncScanOptions extends AsyncClientOptions {
  protocol?: Protocol;
  preset?: ScanPreset;
  projectPath?: string;
}

export interface FreePortOptions extends ClientOptions {
  /** Defaults to true. Set false to explicitly terminate the owning process. */
  dryRun?: boolean;
  /** Optional preview token returned by a prior dry run to bind execution to that snapshot. */
  previewId?: string;
}

export interface AsyncFreePortOptions extends AsyncClientOptions {
  /** Defaults to true. Set false to explicitly terminate the owning process. */
  dryRun?: boolean;
  /** Optional preview token returned by a prior dry run to bind execution to that snapshot. */
  previewId?: string;
}

export interface CleanProjectOptions extends FreePortOptions {
  /** Explicit project directory; defaults to cwd or the caller's working directory. */
  projectPath?: string;
  /**
   * Limit cleanup to these ports. Omitted or `null` discovers project listeners.
   * An explicitly empty array throws a TypeError; it never means "discover everything".
   */
  ports?: number[] | null;
}

export interface AsyncCleanProjectOptions extends AsyncFreePortOptions {
  projectPath?: string;
  ports?: number[] | null;
}

export interface WaitOptions extends ClientOptions {
  /** Desired TCP state. Defaults to "open". */
  state?: WaitState;
  /** Maximum time to wait in milliseconds. Defaults to 30000. */
  waitTimeout?: number;
  /** Polling interval in milliseconds. Defaults to 250. */
  interval?: number;
}

export interface AsyncWaitOptions extends AsyncClientOptions {
  state?: WaitState;
  waitTimeout?: number;
  interval?: number;
}

export interface FindFreeOptions extends ClientOptions {
  /** First candidate port (inclusive). Defaults to 3000. */
  from?: number;
  /** Last candidate port (inclusive). Defaults to 3100. */
  to?: number;
}

export interface AsyncFindFreeOptions extends AsyncClientOptions {
  from?: number;
  to?: number;
}

export const DEFAULT_TIMEOUT: number;

export class PortDetectiveError extends Error {}
export class PortDetectiveNotFoundError extends PortDetectiveError {}
export class PortDetectiveVersionError extends PortDetectiveError {}
export class PortDetectiveExecutionError extends PortDetectiveError {
  returncode: number;
  stderr: string;
  stdout: string;
}

export function inspectPort(port: number, options?: InspectOptions): PortDiagnostic;
/**
 * Scan the given ports, or the default development ports when `ports` is omitted or
 * `null`. An explicitly empty array throws a TypeError instead of scanning defaults.
 */
export function scanPorts(ports?: number[] | null, options?: ScanOptions): PortDiagnostic[];
export function freePort(port: number, options?: FreePortOptions): KillResult;
export function cleanProject(options?: CleanProjectOptions): PortDiagnostic[];
export function waitForPort(port: number, options?: WaitOptions): PortDiagnostic;
export function findFreePort(options?: FindFreeOptions): FreePortResult;
export function doctor(options?: ClientOptions): DoctorReport;

export function asyncInspectPort(port: number, options?: AsyncInspectOptions): Promise<PortDiagnostic>;
/** @see {@link scanPorts} — an explicitly empty array also throws a TypeError. */
export function asyncScanPorts(ports?: number[] | null, options?: AsyncScanOptions): Promise<PortDiagnostic[]>;
export function asyncFreePort(port: number, options?: AsyncFreePortOptions): Promise<KillResult>;
export function asyncCleanProject(options?: AsyncCleanProjectOptions): Promise<PortDiagnostic[]>;
export function asyncWaitForPort(port: number, options?: AsyncWaitOptions): Promise<PortDiagnostic>;
export function asyncFindFreePort(options?: AsyncFindFreeOptions): Promise<FreePortResult>;
export function asyncDoctor(options?: AsyncClientOptions): Promise<DoctorReport>;
