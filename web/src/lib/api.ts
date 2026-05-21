/**
 * API client for the tt backend.
 *
 * Backend always returns a single error envelope on non-2xx:
 *   { "error": { "code": "validation_failed", "message": "...", "details": {...} } }
 *
 * Successful responses are either JSON or 204 No Content.
 */

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
  };
}

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly details?: Record<string, unknown>;

  constructor(code: string, message: string, status: number, details?: Record<string, unknown>) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

const API_BASE = '/api/v1';

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const url =
    path.startsWith('http') || path.startsWith(API_BASE)
      ? path
      : `${API_BASE}${path.startsWith('/') ? path : `/${path}`}`;

  const headers = new Headers(init?.headers);
  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json');
  }
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(url, { ...init, headers });

  if (response.status === 204) {
    return undefined as T;
  }

  if (!response.ok) {
    let envelope: ApiErrorEnvelope | undefined;
    try {
      envelope = (await response.json()) as ApiErrorEnvelope;
    } catch {
      // body wasn't JSON; fall through to generic error
    }
    if (envelope?.error?.code) {
      throw new ApiError(
        envelope.error.code,
        envelope.error.message,
        response.status,
        envelope.error.details,
      );
    }
    throw new ApiError(
      'http_error',
      `HTTP ${response.status} ${response.statusText}`,
      response.status,
    );
  }

  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}
