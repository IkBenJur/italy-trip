/**
 * Saving an original.
 *
 * /photos/:id/original sits behind RequireAuth, so a plain <a href> would send
 * no Authorization header and get a 401. The bytes are fetched through
 * apiClient instead, and then handed to the browser as a download — that also
 * means an access token that expired between opening the album and clicking
 * download gets transparently refreshed and retried, same as any other request.
 *
 * That Authorization header makes this a CORS request, which is why the API
 * serves the bytes itself rather than redirecting to the bucket: a CORS fetch
 * that follows a cross-origin redirect sends Origin: null and lands on an
 * origin with no Access-Control-Allow-Origin, and the browser drops it.
 */
import { apiClient } from "~/lib/apiClient";
import { photoService } from "~/services/photo.service";

export function downloadFilename(takenAt: string): string {
  const date = new Date(takenAt);
  if (Number.isNaN(date.getTime())) return "italy-photo.jpg";

  const pad = (value: number) => String(value).padStart(2, "0");
  return (
    `italy-${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}` +
    `-${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}.jpg`
  );
}

export async function downloadPhoto(photo: { id: string; taken_at: string }): Promise<void> {
  const blob = await apiClient.getBlob(photoService.originalPath(photo.id));
  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = downloadFilename(photo.taken_at);
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    // Give the browser a moment to start the save before pulling the URL away.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
}

/**
 * Downloads every photo in the current event as one zip, built server-side —
 * one authenticated request instead of one per photo (which used to be fired
 * 300ms apart because Safari drops most of a burst of concurrent downloads).
 *
 * There's no real "X / Y" progress to report: the zip has no known
 * Content-Length while it's streaming, so onProgress only reports which of
 * the three phases the download is in.
 */
export async function downloadAllPhotos(
  onProgress?: (state: "starting" | "downloading" | "done") => void,
): Promise<void> {
  onProgress?.("starting");
  const res = await apiClient.getAuthed(photoService.downloadAllPath());

  onProgress?.("downloading");
  const blob = await res.blob();

  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "italy-trip-photos.zip";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
  onProgress?.("done");
}
