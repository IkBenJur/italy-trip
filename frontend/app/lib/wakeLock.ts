/**
 * Keeps the screen awake during a slideshow where the browser supports it.
 * Safari's support is uneven, so every path here degrades silently rather than
 * surfacing an error for something purely nice-to-have.
 */
interface WakeLockSentinelLike {
  release: () => Promise<void>;
  released?: boolean;
}

export async function requestWakeLock(): Promise<WakeLockSentinelLike | null> {
  const wakeLock = (navigator as Navigator & { wakeLock?: { request: (type: "screen") => Promise<WakeLockSentinelLike> } }).wakeLock;
  if (!wakeLock) return null;

  try {
    return await wakeLock.request("screen");
  } catch {
    return null;
  }
}

export async function releaseWakeLock(sentinel: WakeLockSentinelLike | null): Promise<void> {
  if (!sentinel || sentinel.released) return;
  try {
    await sentinel.release();
  } catch {
    // Nothing to do: the lock is gone either way.
  }
}
