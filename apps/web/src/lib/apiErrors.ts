import type { RunNodeResponse } from "./api";

export function apiErrorMessage(value: unknown, fallback: string) {
  if (isErrorPayload(value)) {
    return value.error;
  }
  if (isRunNodeErrorPayload(value)) {
    return value.job.error_message || value.job.error_code || fallback;
  }
  return fallback;
}

export function isErrorPayload(value: unknown): value is { error: string } {
  return (
    value !== null &&
    typeof value === "object" &&
    "error" in value &&
    typeof (value as { error?: unknown }).error === "string"
  );
}

export function isRunNodeErrorPayload(
  value: unknown,
): value is RunNodeResponse {
  return (
    value !== null &&
    typeof value === "object" &&
    "job" in value &&
    typeof (value as { job?: unknown }).job === "object"
  );
}
