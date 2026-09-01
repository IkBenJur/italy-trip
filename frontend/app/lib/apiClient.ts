import { env } from "~/lib/env";
import { getToken } from "~/lib/auth";

/**
 * ApiError keeps the HTTP status alongside the message. The status matters:
 * 423 (the event is still running) has to be told apart from a 401, because the
 * first means "wait" and the second means "log in again".
 */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
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

  // Add getIsTokenExpired

  /** 4xx other than 423 will never succeed on a retry. */
  get isPermanent() {
    return this.status >= 400 && this.status < 500 && this.status !== 423;
  }
}

class ApiClient {
  private baseUrl: string;

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
      throw new ApiError(res.status, payload?.error ?? `HTTP ${res.status}: ${res.statusText}`);
    }

    if (res.status === 204) {
      return undefined as T;
    }

    return res.json() as Promise<T>;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers: { "Content-Type": "application/json", ...this.authHeaders() },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });

    // TODO Add token expired check.
    // If token expired. Retrieve new token.
    // Refetch with new token.
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
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: "POST",
      headers: this.authHeaders(),
      body: form,
      signal,
    });

    return this.parse<T>(res);
  }

  /** The absolute URL for a path, for links the browser follows itself. */
  url(path: string): string {
    return `${this.baseUrl}${path}`;
  }
}

export const apiClient = new ApiClient(env.API_URL);
