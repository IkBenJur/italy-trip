/**
 * Saving an original.
 *
 * /photos/:id/original sits behind RequireAuth, so a plain <a href> would send
 * no Authorization header and get a 401. The bytes are fetched with the token
 * instead — the server 302s to a presigned URL and fetch follows it — and then
 * handed to the browser as a download.
 */
import { getToken } from "~/lib/auth";
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
  const token = getToken();
  const res = await fetch(photoService.originalUrl(photo.id), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });

  if (!res.ok) {
    throw new Error(`Could not download the photo (HTTP ${res.status})`);
  }

  const blob = await res.blob();
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
 * Downloads every photo, one at a time. Firing sixty at once makes Safari drop
 * most of them.
 */
export async function downloadAll(
  photos: Array<{ id: string; taken_at: string }>,
  onProgress?: (done: number, total: number) => void,
): Promise<void> {
  for (const [index, photo] of photos.entries()) {
    await downloadPhoto(photo);
    onProgress?.(index + 1, photos.length);
    await new Promise((resolve) => setTimeout(resolve, 300));
  }
}
