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
}

export interface KillResult {
  port: number;
  dry_run: boolean;
  message: string;
  diagnostic?: PortDiagnostic | null;
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

export interface FreePortOptions extends ClientOptions {
  /** Defaults to true. Set false to explicitly terminate the owning process. */
  dryRun?: boolean;
}

export interface AsyncFreePortOptions extends AsyncClientOptions {
  /** Defaults to true. Set false to explicitly terminate the owning process. */
  dryRun?: boolean;
}

export interface CleanProjectOptions extends FreePortOptions {
  /** Explicit project directory; defaults to cwd or the caller's working directory. */
  projectPath?: string;
  /** Limit cleanup to these ports; omitted or empty discovers project listeners. */
  ports?: number[] | null;
}
export interface AsyncCleanProjectOptions extends AsyncFreePortOptions {
  projectPath?: string;
  ports?: number[] | null;
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

export function inspectPort(port: number, options?: ClientOptions): PortDiagnostic;
export function scanPorts(ports?: number[] | null, options?: ClientOptions): PortDiagnostic[];
export function freePort(port: number, options?: FreePortOptions): KillResult;
export function cleanProject(options?: CleanProjectOptions): PortDiagnostic[];

export function asyncInspectPort(port: number, options?: AsyncClientOptions): Promise<PortDiagnostic>;
export function asyncScanPorts(ports?: number[] | null, options?: AsyncClientOptions): Promise<PortDiagnostic[]>;
export function asyncFreePort(port: number, options?: AsyncFreePortOptions): Promise<KillResult>;
export function asyncCleanProject(options?: AsyncCleanProjectOptions): Promise<PortDiagnostic[]>;
