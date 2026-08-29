import { describe, expect, it } from "vitest";
import { normalizeApiUrl } from "~/lib/env";

describe("normalizeApiUrl", () => {
  it.each([
    // The Railway shape: a bare hostname, which fetch() would treat as relative.
    ["italy-api-production.up.railway.app", "https://italy-api-production.up.railway.app"],
    ["https://italy-api-production.up.railway.app", "https://italy-api-production.up.railway.app"],
    ["https://italy-api-production.up.railway.app/", "https://italy-api-production.up.railway.app"],
    ["https://italy-api-production.up.railway.app///", "https://italy-api-production.up.railway.app"],
    ["  italy-api-production.up.railway.app  ", "https://italy-api-production.up.railway.app"],

    // Local development keeps http.
    ["localhost:8080", "http://localhost:8080"],
    ["http://localhost:8080", "http://localhost:8080"],
    ["127.0.0.1:8080", "http://127.0.0.1:8080"],

    // Unset or blank falls back rather than producing a broken base URL.
    [undefined, "http://localhost:8080"],
    ["", "http://localhost:8080"],
    ["   ", "http://localhost:8080"],
  ])("normalizes %s", (input, expected) => {
    expect(normalizeApiUrl(input)).toBe(expected);
  });

  it("produces a base URL that concatenates cleanly with a path", () => {
    // The failure this guards against: "host/auth/login" is a relative URL.
    const base = normalizeApiUrl("italy-api-production.up.railway.app");
    const url = new URL(`${base}/auth/login`);

    expect(url.origin).toBe("https://italy-api-production.up.railway.app");
    expect(url.pathname).toBe("/auth/login");
  });
});
