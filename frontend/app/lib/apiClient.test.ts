import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "~/lib/apiClient";
import { clearToken, getToken, setToken } from "~/lib/auth";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("ApiClient token refresh", () => {
  let client: ApiClient;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    clearToken();
    setToken("stale-token");
    client = new ApiClient("http://api.test");
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    clearToken();
  });

  it("retries once with a refreshed token after a token_expired 401", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { token: "fresh-token", user: { id: "1", email: "a@b.com" } }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));

    const result = await client.get<{ ok: boolean }>("/events/current");

    expect(result).toEqual({ ok: true });
    expect(getToken()).toBe("fresh-token");
    expect(fetchMock).toHaveBeenCalledTimes(3);

    // Second call is the refresh, sent with credentials for the cookie.
    const refreshCall = fetchMock.mock.calls[1]!;
    expect(refreshCall[0]).toBe("http://api.test/auth/refresh");
    expect(refreshCall[1]).toMatchObject({ method: "POST", credentials: "include" });

    // Third call (the retry) carries the refreshed token, not the stale one.
    const retryCall = fetchMock.mock.calls[2]!;
    const retryHeaders = retryCall[1].headers as Record<string, string>;
    expect(retryHeaders.Authorization).toBe("Bearer fresh-token");
  });

  it("does not retry a 401 that isn't a token_expired error", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { error: "invalid token" }));

    await expect(client.get("/events/current")).rejects.toMatchObject({
      status: 401,
      code: undefined,
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("clears the token and surfaces the original 401 when refresh itself fails", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "invalid refresh token" }));

    const error = await client.get("/events/current").catch((e) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).isTokenExpired).toBe(true);
    expect(getToken()).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("coalesces concurrent 401s onto a single refresh call", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { token: "fresh-token", user: { id: "1", email: "a@b.com" } }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }))
      .mockResolvedValueOnce(jsonResponse(200, { ok: true }));

    const [a, b] = await Promise.all([
      client.get<{ ok: boolean }>("/events/current"),
      client.get<{ ok: boolean }>("/events/current"),
    ]);

    expect(a).toEqual({ ok: true });
    expect(b).toEqual({ ok: true });

    const refreshCalls = fetchMock.mock.calls.filter((call) => call[0] === "http://api.test/auth/refresh");
    expect(refreshCalls).toHaveLength(1);
  });

  it("getBlob retries once with a refreshed token after a token_expired 401", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { token: "fresh-token", user: { id: "1", email: "a@b.com" } }))
      .mockResolvedValueOnce(new Response(new Blob(["jpeg"]), { status: 200 }));

    const blob = await client.getBlob("/photos/p1/original");

    expect(blob).toBeInstanceOf(Blob);
    expect(fetchMock).toHaveBeenCalledTimes(3);

    const retryCall = fetchMock.mock.calls[2]!;
    const retryHeaders = retryCall[1].headers as Record<string, string>;
    expect(retryHeaders.Authorization).toBe("Bearer fresh-token");
  });

  it("getBlob surfaces a non-retryable error as ApiError", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(423, { error: "the event is still running" }));

    await expect(client.getBlob("/photos/p1/original")).rejects.toMatchObject({
      status: 423,
      message: "the event is still running",
    });
  });
});
