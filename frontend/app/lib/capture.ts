/**
 * Turning a live <video> frame into an uploadable JPEG.
 *
 * A frame drawn from a video element is a raw bitmap already in display
 * orientation: no EXIF, no HEIC, no orientation flag to correct. That is the
 * whole reason the app captures in-page rather than through <input capture>.
 *
 * The one thing that must not be got wrong is the source size. videoWidth and
 * videoHeight are read here, at capture time, and never taken from the
 * videoConstraints — those are a request the browser is free to ignore, and iOS
 * is known to swap track dimensions on rotation.
 */

export interface CaptureResult {
  blob: Blob;
  /** The real pixel dimensions of the captured frame. */
  width: number;
  height: number;
  /** RFC3339 with offset. There is no EXIF, so capture time is recorded here. */
  takenAt: string;
}

/** The minimal shape of a video element this module needs. */
export interface CaptureSource {
  videoWidth: number;
  videoHeight: number;
  readyState?: number;
}

export interface CaptureOptions {
  /** JPEG quality. 0.92 puts a 1080p frame at roughly 200-400 KB. */
  quality?: number;
  /** Injectable for tests; defaults to a real detached canvas. */
  createCanvas?: () => HTMLCanvasElement;
  /** Injectable for tests. */
  now?: () => Date;
}

export const DEFAULT_QUALITY = 0.92;

/** HTMLMediaElement.HAVE_CURRENT_DATA — the first state with a frame to draw. */
const HAVE_CURRENT_DATA = 2;

export class CaptureError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CaptureError";
  }
}

/**
 * Reads the frame dimensions, refusing anything that would produce a blank
 * image. A stream that has not started yet reports 0x0, and silently uploading
 * a black rectangle is far worse than failing the shutter press.
 */
export function frameSize(video: CaptureSource): { width: number; height: number } {
  const width = Math.floor(video.videoWidth);
  const height = Math.floor(video.videoHeight);

  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    throw new CaptureError(
      `camera is not ready yet (frame is ${video.videoWidth}x${video.videoHeight})`,
    );
  }

  if (video.readyState !== undefined && video.readyState < HAVE_CURRENT_DATA) {
    throw new CaptureError("camera is not ready yet (no frame available)");
  }

  return { width, height };
}

/** Formats a Date as RFC3339 with the device's own UTC offset. */
export function toRfc3339(date: Date): string {
  const pad = (value: number, size = 2) => String(Math.abs(value)).padStart(size, "0");

  // getTimezoneOffset is minutes *behind* UTC, so the sign is inverted.
  const offsetMinutes = -date.getTimezoneOffset();
  const sign = offsetMinutes >= 0 ? "+" : "-";
  const offset = `${sign}${pad(Math.floor(Math.abs(offsetMinutes) / 60))}:${pad(Math.abs(offsetMinutes) % 60)}`;

  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}${offset}`
  );
}

/**
 * Draws the current frame to a canvas sized to match it exactly, and encodes it
 * as JPEG.
 */
export async function captureFrame(
  video: CaptureSource,
  options: CaptureOptions = {},
): Promise<CaptureResult> {
  const { width, height } = frameSize(video);

  const canvas = (options.createCanvas ?? (() => document.createElement("canvas")))();
  canvas.width = width;
  canvas.height = height;

  const context = canvas.getContext("2d");
  if (!context) {
    throw new CaptureError("could not get a 2d drawing context");
  }

  context.drawImage(video as unknown as CanvasImageSource, 0, 0, width, height);

  const blob = await toJpegBlob(canvas, options.quality ?? DEFAULT_QUALITY);
  if (!blob || blob.size === 0) {
    throw new CaptureError("the browser produced an empty image");
  }

  return {
    blob,
    width,
    height,
    takenAt: toRfc3339((options.now ?? (() => new Date()))()),
  };
}

function toJpegBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob | null> {
  return new Promise((resolve, reject) => {
    try {
      canvas.toBlob((blob) => resolve(blob), "image/jpeg", quality);
    } catch (error) {
      reject(error instanceof Error ? error : new CaptureError(String(error)));
    }
  });
}
