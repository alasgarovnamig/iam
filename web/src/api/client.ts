import axios, { AxiosError, type AxiosInstance, type InternalAxiosRequestConfig } from 'axios';

/**
 * Token handling.
 *
 * The access token lives in memory only — never in localStorage/sessionStorage,
 * so it does not survive an XSS payload that reads storage, and it disappears
 * when the tab closes. The refresh token is never visible to JavaScript at all:
 * the API keeps it in a HttpOnly, SameSite=Strict cookie (we opt in with the
 * `X-Auth-Mode: cookie` header). Session continuity across reloads therefore
 * comes from calling /auth/refresh with that cookie, not from stored tokens.
 */
let accessToken: string | null = null;
let onUnauthenticated: (() => void) | null = null;

export function setAccessToken(token: string | null): void {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

export function setUnauthenticatedHandler(handler: (() => void) | null): void {
  onUnauthenticated = handler;
}

export const API_BASE_URL: string =
  (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? 'http://localhost:8090';

export const api: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  // Required for the HttpOnly refresh cookie to travel to the API origin.
  withCredentials: true,
  headers: { 'Content-Type': 'application/json', 'X-Auth-Mode': 'cookie' },
  timeout: 30_000,
});

api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  if (accessToken) {
    config.headers.set('Authorization', `Bearer ${accessToken}`);
  }
  return config;
});

/** RFC 7807 problem document returned by the API on errors. */
export interface ProblemDetail {
  type: string;
  title: string;
  status: number;
  detail: string;
  code: string;
  request_id?: string;
  errors?: { field: string; message: string }[];
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly detail: string;
  readonly fieldErrors: { field: string; message: string }[];
  readonly requestId?: string;

  constructor(problem: ProblemDetail) {
    super(problem.detail || problem.title);
    this.name = 'ApiError';
    this.status = problem.status;
    this.code = problem.code;
    this.detail = problem.detail;
    this.fieldErrors = problem.errors ?? [];
    this.requestId = problem.request_id;
  }
}

function toApiError(error: AxiosError<ProblemDetail>): ApiError {
  if (error.response?.data && typeof error.response.data === 'object' && 'code' in error.response.data) {
    return new ApiError(error.response.data);
  }
  return new ApiError({
    type: 'about:blank',
    title: 'Network error',
    status: error.response?.status ?? 0,
    detail: error.message || 'The API could not be reached.',
    code: error.response?.status ? 'HTTP_ERROR' : 'NETWORK_ERROR',
  });
}

/**
 * Refresh-on-401 with single-flight: concurrent requests that hit an expired
 * access token share one refresh call and are then replayed once.
 */
let refreshInFlight: Promise<string> | null = null;

async function refreshAccessToken(): Promise<string> {
  const response = await axios.post<{ data: { access_token: string } }>(
    `${API_BASE_URL}/api/v1/auth/refresh`,
    {},
    { withCredentials: true, headers: { 'X-Auth-Mode': 'cookie', 'Content-Type': 'application/json' } },
  );
  return response.data.data.access_token;
}

api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError<ProblemDetail>) => {
    const original = error.config as InternalAxiosRequestConfig & { _retried?: boolean };
    const isAuthCall = original?.url?.includes('/auth/refresh') || original?.url?.includes('/auth/login');

    if (error.response?.status === 401 && original && !original._retried && !isAuthCall) {
      original._retried = true;
      try {
        refreshInFlight = refreshInFlight ?? refreshAccessToken();
        const token = await refreshInFlight;
        refreshInFlight = null;
        setAccessToken(token);
        return api(original);
      } catch {
        refreshInFlight = null;
        setAccessToken(null);
        onUnauthenticated?.();
      }
    }
    return Promise.reject(toApiError(error));
  },
);

// ─── Response envelopes ─────────────────────────────────────────────────────

export interface Envelope<T> {
  data: T;
  meta: { request_id: string; timestamp: string };
}

export interface PageInfo {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface ListEnvelope<T> {
  data: T[];
  pagination: PageInfo;
  meta: { request_id: string; timestamp: string };
}

export async function getData<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const response = await api.get<Envelope<T>>(url, { params });
  return response.data.data;
}

export async function getList<T>(
  url: string,
  params?: Record<string, unknown>,
): Promise<{ items: T[]; pagination: PageInfo }> {
  const response = await api.get<ListEnvelope<T>>(url, { params });
  return { items: response.data.data ?? [], pagination: response.data.pagination };
}

export async function postData<T>(url: string, body?: unknown, headers?: Record<string, string>): Promise<T> {
  const response = await api.post<Envelope<T>>(url, body ?? {}, { headers });
  return response.data?.data;
}

/** POSTs a search body and returns the paginated list envelope. */
export async function postList<T>(
  url: string,
  body: unknown,
): Promise<{ items: T[]; pagination: PageInfo }> {
  const response = await api.post<ListEnvelope<T>>(url, body);
  return { items: response.data.data ?? [], pagination: response.data.pagination };
}

export async function patchData<T>(url: string, body: unknown): Promise<T> {
  const response = await api.patch<Envelope<T>>(url, body);
  return response.data.data;
}

export async function deleteData(url: string): Promise<void> {
  await api.delete(url);
}

/** Builds the filter[field]=value query the API expects. */
export function filterParams(filters: Record<string, string | undefined>): Record<string, string> {
  const params: Record<string, string> = {};
  for (const [field, value] of Object.entries(filters)) {
    if (value) params[`filter[${field}]`] = value;
  }
  return params;
}
