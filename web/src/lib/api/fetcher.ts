import type { ErrorCode } from "./gen/model";

/** An API error in the shape of SPEC.md section 8. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | "network";
  readonly fields: Record<string, string>;

  constructor(status: number, code: ErrorCode | "network", message: string, fields: Record<string, string> = {}) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

type ErrorBody = { error?: { code?: ErrorCode; message?: string; fields?: Record<string, string> } };

/** Called when any request comes back 401, so the shell can send the user to sign in. */
let onUnauthorized: (() => void) | undefined;
export function setUnauthorizedHandler(fn: (() => void) | undefined) {
  onUnauthorized = fn;
}

/** The fetch used by the generated client. Returns the parsed body or throws ApiError. */
export async function apiFetch<T>(url: string, init: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { ...init, credentials: "same-origin" });
  } catch {
    throw new ApiError(0, "network", "Cannot reach the Gokku server.");
  }

  const text = res.status === 204 ? "" : await res.text();
  const body: unknown = text ? JSON.parse(text) : undefined;

  if (!res.ok) {
    const err = (body as ErrorBody | undefined)?.error;
    if (res.status === 401) onUnauthorized?.();
    throw new ApiError(res.status, err?.code ?? "internal", err?.message ?? res.statusText, err?.fields);
  }
  return body as T;
}
