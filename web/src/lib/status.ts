import type { StatusKind } from "~/components/app/status-pill";
import type { AppStatus, ServiceStatus } from "./api/gen/model";
import { strings } from "./strings";

export function appStatusLabel(s: AppStatus): string {
  return strings.appStatus[s];
}

export const serviceStatusKind: Record<ServiceStatus, StatusKind> = {
  running: "running",
  stopped: "stopped",
  missing: "failed",
};

/** The datastore's display name: "postgres" becomes "Postgres". */
export function datastoreName(type: string): string {
  return strings.datastores[type] ?? type;
}

/** The first 7 characters of a commit, as git prints it. */
export function shortSha(sha: string): string {
  return sha.slice(0, 7);
}
