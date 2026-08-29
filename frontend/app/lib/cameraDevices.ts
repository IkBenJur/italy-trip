/**
 * Which way the camera points, and whether there is a second one to point the
 * other way.
 *
 * The device probe deliberately runs *after* the stream is live. Before a page
 * has been granted the camera, enumerateDevices() returns entries with blank
 * labels and browsers are free to collapse them — Safari in particular reports
 * a single placeholder videoinput. Probing on mount would therefore hide the
 * flip button on exactly the phones that need it.
 */

export type Facing = "user" | "environment";

const FACING_KEY = "camera_facing";

export const DEFAULT_FACING: Facing = "environment";

/**
 * The app is prerendered to a static index.html at build time, so this has to
 * survive there being no window at all.
 */
function storage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    // Safari in private mode can throw on access rather than return null.
    return null;
  }
}

function isFacing(value: unknown): value is Facing {
  return value === "user" || value === "environment";
}

/** The camera last used, or the rear one for a device we have not seen before. */
export function readFacing(): Facing {
  let stored: string | null = null;
  try {
    stored = storage()?.getItem(FACING_KEY) ?? null;
  } catch {
    return DEFAULT_FACING;
  }
  return isFacing(stored) ? stored : DEFAULT_FACING;
}

export function writeFacing(facing: Facing): void {
  try {
    storage()?.setItem(FACING_KEY, facing);
  } catch {
    // A full or unavailable quota is not worth failing a camera flip over.
  }
}

/**
 * Whether the device has more than one camera, and so anything to flip to.
 *
 * Answers false for every failure mode — no mediaDevices, a browser that
 * refuses to enumerate, a rejected promise — because a flip button that cannot
 * flip is worse than no button.
 */
export async function hasMultipleCameras(): Promise<boolean> {
  if (
    typeof navigator === "undefined" ||
    typeof navigator.mediaDevices === "undefined" ||
    typeof navigator.mediaDevices.enumerateDevices !== "function"
  ) {
    return false;
  }

  try {
    const devices = await navigator.mediaDevices.enumerateDevices();
    return devices.filter((device) => device.kind === "videoinput").length > 1;
  } catch {
    return false;
  }
}
