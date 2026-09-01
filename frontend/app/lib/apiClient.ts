import { env } from "~/lib/env";
import { getToken, setToken, clearToken } from "~/lib/auth";
import type { AuthResponse } from "~/types/user.types";

/**
 * ApiError keeps the HTTP status alongside the message. The status matters:
 * 423 (the event is still running) has to be told apart from a 401, because the
 * first means "wait" and the second means "log in again".
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }

  get isLocked() {
    return this.status === 423;
  }

  get isUnauthorized() {
    return this.status === 401;
  }

  get isNotFound() {
    return this.status === 404;
  }

  /** A 401 caused specifically by an expired access token, as opposed to one
   * that will never succeed (missing/malformed/forged). Only this kind is
   * worth retrying after a token refresh. */
  get isTokenExpired() {
    return this.status === 401 && this.code === "token_expired";
  }

  /** 4xx other than 423 will never succeed on a retry. */
  get isPermanent() {
    return this.status >= 400 && this.status < 500 && this.status !== 423;
  }
}

export class ApiClient {
  private baseUrl: string;

  /** Coalesces concurrent 401s onto one refresh call instead of each one
   * racing the backend's token rotation and stepping on each other. */
  private refreshPromise: Promise<string> | null = null;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  private authHeaders(): Record<string, string> {
    const token = getToken();
    return token ? { Authorization: `Bearer ${token}` } : {};
  }

  private async parse<T>(res: Response): Promise<T> {
    if (!res.ok) {
      const payload = await res.json().catch(() => null);
      throw new ApiError(res.status, payload?.error ?? `HTTP ${res.status}: ${res.statusText}`, payload?.code);
    }

    if (res.status === 204) {
      return undefined as T;
    }

    return res.json() as Promise<T>;
  }

  /**
   * Exchanges the refresh-token cookie (sent automatically via
   * `credentials: "include"`) for a new access token. The server rotates the
   * cookie on its response, so this never reads or writes it directly.
   */
  private refreshToken(): Promise<string> {
    if (!this.refreshPromise) {
      this.refreshPromise = fetch(`${this.baseUrl}/auth/refresh`, {
        method: "POST",
        credentials: "include",
      })
        .then((res) => this.parse<AuthResponse>(res))
        .then((data) => {
          setToken(data.token);
          return data.token;
        })
        .finally(() => {
          this.refreshPromise = null;
        });
    }
    return this.refreshPromise;
  }

  /**
   * Runs one attempt, and on a token-expired 401 refreshes and retries once.
   * `buildInit` is re-invoked on the retry so the Authorization header picks
   * up the refreshed token rather than replaying the stale one.
   */
  private async fetchWithRefresh(
    path: string,
    buildInit: (authHeaders: Record<string, string>) => RequestInit,
    allowRetry = true,
  ): Promise<Response> {
    const res = await fetch(`${this.baseUrl}${path}`, { credentials: "include", ...buildInit(this.authHeaders()) });

    if (res.status !== 401 || !allowRetry) {
      return res;
    }

    const payload = await res.clone().json().catch(() => null);
    if (payload?.code !== "token_expired") {
      return res;
    }

    try {
      await this.refreshToken();
    } catch {
      clearToken();
      return res;
    }

    return this.fetchWithRefresh(path, buildInit, false);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await this.fetchWithRefresh(path, (authHeaders) => ({
      method,
      headers: { "Content-Type": "application/json", ...authHeaders },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }));

    return this.parse<T>(res);
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  put<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("PUT", path, body);
  }

  delete<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  /**
   * postForm sends multipart/form-data. Content-Type is deliberately left off so
   * the browser generates the multipart boundary itself.
   */
  async postForm<T>(path: string, form: FormData, signal?: AbortSignal): Promise<T> {
    const res = await this.fetchWithRefresh(path, (authHeaders) => ({
      method: "POST",
      headers: authHeaders,
      body: form,
      signal,
    }));

    return this.parse<T>(res);
  }

  /** The absolute URL for a path, for links the browser follows itself. */
  url(path: string): string {
    return `${this.baseUrl}${path}`;
  }
}

export const apiClient = new ApiClient(env.API_URL);
