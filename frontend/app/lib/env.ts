const DEFAULT_API_URL = "http://localhost:8080";

/**
 * Railway's domain variables are bare hostnames, so VITE_API_URL is easily set
 * to "italy-api.up.railway.app" rather than a full URL.
 *
 * Left as-is that is a *relative* path as far as fetch() is concerned, so every
 * API call would quietly go to the frontend's own origin and 404, rather than
 * reaching the backend at all. Vite bakes this value in at build time, so
 * getting it wrong means rebuilding, not just restarting.
 */
export function normalizeApiUrl(value: string | undefined): string {
  const trimmed = value?.trim();
  if (!trimmed) return DEFAULT_API_URL;

  const hasScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed);
  const withScheme = hasScheme ? trimmed : `${schemeFor(trimmed)}://${trimmed}`;

  // A trailing slash would double up, since every path already starts with one.
  return withScheme.replace(/\/+$/, "");
}

/** Local development is the only place that is not HTTPS. */
function schemeFor(hostPort: string): string {
  const host = hostPort.split("/")[0]!.split(":")[0]!.toLowerCase();
  return host === "localhost" || host === "127.0.0.1" ? "http" : "https";
}

export const env = {
  API_URL: normalizeApiUrl(import.meta.env.VITE_API_URL),
};
