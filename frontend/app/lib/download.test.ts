import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "~/lib/apiClient";
import { downloadAll, downloadFilename, downloadPhoto } from "~/lib/download";

const photo = { id: "p1", taken_at: "2026-09-06T14:30:15+02:00" };

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  window.localStorage.setItem("auth_token", "a-token");

  // jsdom implements neither of these.
  Object.defineProperty(URL, "createObjectURL", {
    value: vi.fn(() => "blob:fake-url"),
    configurable: true,
  });
  Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("downloadFilename", () => {
  it("names the file after when the photo was taken", () => {
    expect(downloadFilename("2026-09-06T14:30:15+02:00")).toMatch(
      /^italy-20260906-\d{6}\.jpg$/,
    );
  });

  it("falls back rather than producing a broken name", () => {
    expect(downloadFilename("not a date")).toBe("italy-photo.jpg");
  });
});

describe("downloadPhoto", () => {
  it("sends the bearer token, which a plain link could not", async () => {
    const fetchMock = vi.fn(async () => new Response(new Blob(["jpeg"]), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    await downloadPhoto(photo);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/photos/p1/original");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer a-token");
    expect(click).toHaveBeenCalledTimes(1);

    vi.unstubAllGlobals();
  });

  it("throws on a failed download rather than saving an error page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(423, { error: "the event is still running" })),
    );

    const error = await downloadPhoto(photo).catch((e) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(423);

    vi.unstubAllGlobals();
  });

  it("transparently refreshes an expired token instead of failing the download", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(401, { error: "token expired", code: "token_expired" }))
      .mockResolvedValueOnce(jsonResponse(200, { token: "fresh-token", user: { id: "1", email: "a@b.com" } }))
      .mockResolvedValueOnce(new Response(new Blob(["jpeg"]), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    await downloadPhoto(photo);

    expect(fetchMock).toHaveBeenCalledTimes(3);
    const retryHeaders = fetchMock.mock.calls[2]![1].headers as Record<string, string>;
    expect(retryHeaders.Authorization).toBe("Bearer fresh-token");

    vi.unstubAllGlobals();
  });
});

describe("downloadAll", () => {
  it("downloads one at a time and reports progress", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(new Blob(["jpeg"]), { status: 200 })),
    );
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    const progress: Array<[number, number]> = [];
    await downloadAll(
      [
        { id: "p1", taken_at: "2026-09-06T10:00:00+02:00" },
        { id: "p2", taken_at: "2026-09-07T10:00:00+02:00" },
        { id: "p3", taken_at: "2026-09-08T10:00:00+02:00" },
      ],
      (done, total) => progress.push([done, total]),
    );

    expect(progress).toEqual([
      [1, 3],
      [2, 3],
      [3, 3],
    ]);

    vi.unstubAllGlobals();
  });
});
