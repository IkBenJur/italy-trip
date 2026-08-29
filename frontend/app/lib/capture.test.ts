import { describe, expect, it, vi } from "vitest";
import {
  captureFrame,
  CaptureError,
  frameSize,
  toRfc3339,
  type CaptureSource,
} from "~/lib/capture";

/** A stand-in for a live <video> element. */
function stubVideo(videoWidth: number, videoHeight: number, readyState = 4): CaptureSource {
  return { videoWidth, videoHeight, readyState };
}

/**
 * jsdom has no canvas implementation, so the test supplies one that records what
 * the capture code asked for.
 */
function stubCanvas() {
  const drawn: unknown[][] = [];
  const canvas = {
    width: 0,
    height: 0,
    toBlobCalls: [] as Array<{ type: string; quality: number }>,
    getContext: () => ({
      drawImage: (...args: unknown[]) => {
        drawn.push(args);
      },
    }),
    toBlob(callback: (blob: Blob | null) => void, type: string, quality: number) {
      canvas.toBlobCalls.push({ type, quality });
      callback(new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0xe0])], { type: "image/jpeg" }));
    },
  };
  return { canvas, drawn };
}

describe("frameSize", () => {
  it("reads the real frame dimensions at capture time", () => {
    expect(frameSize(stubVideo(1920, 1080))).toEqual({ width: 1920, height: 1080 });
  });

  it("reads rotated dimensions rather than assuming the constraints held", () => {
    // iOS can swap track dimensions on rotation; whatever the element reports now
    // is what gets captured.
    expect(frameSize(stubVideo(1080, 1920))).toEqual({ width: 1080, height: 1920 });
  });

  it.each([
    ["stream not started", 0, 0],
    ["zero width", 0, 1080],
    ["zero height", 1920, 0],
    ["negative", -1920, -1080],
    ["NaN", Number.NaN, Number.NaN],
  ])("throws rather than capturing a blank frame: %s", (_label, width, height) => {
    expect(() => frameSize(stubVideo(width, height))).toThrow(CaptureError);
  });

  it("throws when the element has no frame available yet", () => {
    expect(() => frameSize(stubVideo(1920, 1080, 1))).toThrow(/not ready/);
  });
});

describe("captureFrame", () => {
  it("sizes the canvas to exactly the source frame", async () => {
    const { canvas, drawn } = stubCanvas();

    const result = await captureFrame(stubVideo(1920, 1080), {
      createCanvas: () => canvas as unknown as HTMLCanvasElement,
    });

    expect(canvas.width).toBe(1920);
    expect(canvas.height).toBe(1080);
    expect(result.width).toBe(1920);
    expect(result.height).toBe(1080);
    expect(drawn).toHaveLength(1);
    expect(drawn[0].slice(1)).toEqual([0, 0, 1920, 1080]);
  });

  it("encodes JPEG at quality 0.92", async () => {
    const { canvas } = stubCanvas();

    const result = await captureFrame(stubVideo(1280, 720), {
      createCanvas: () => canvas as unknown as HTMLCanvasElement,
    });

    expect(canvas.toBlobCalls).toEqual([{ type: "image/jpeg", quality: 0.92 }]);
    expect(result.blob.type).toBe("image/jpeg");
    expect(result.blob.size).toBeGreaterThan(0);
  });

  it("handles a portrait source", async () => {
    const { canvas } = stubCanvas();

    await captureFrame(stubVideo(1080, 1920), {
      createCanvas: () => canvas as unknown as HTMLCanvasElement,
    });

    expect([canvas.width, canvas.height]).toEqual([1080, 1920]);
  });

  it("throws on a 0x0 source rather than uploading a blank frame", async () => {
    const { canvas } = stubCanvas();

    await expect(
      captureFrame(stubVideo(0, 0), {
        createCanvas: () => canvas as unknown as HTMLCanvasElement,
      }),
    ).rejects.toThrow(CaptureError);

    // Nothing was even attempted.
    expect(canvas.toBlobCalls).toHaveLength(0);
  });

  it("throws when the browser hands back an empty blob", async () => {
    const canvas = {
      width: 0,
      height: 0,
      getContext: () => ({ drawImage: () => {} }),
      toBlob: (callback: (blob: Blob | null) => void) => callback(null),
    };

    await expect(
      captureFrame(stubVideo(1920, 1080), {
        createCanvas: () => canvas as unknown as HTMLCanvasElement,
      }),
    ).rejects.toThrow(/empty image/);
  });

  it("records capture time, since a canvas frame carries no EXIF", async () => {
    const { canvas } = stubCanvas();
    const takenAt = new Date(2026, 8, 6, 14, 30, 15);

    const result = await captureFrame(stubVideo(800, 600), {
      createCanvas: () => canvas as unknown as HTMLCanvasElement,
      now: () => takenAt,
    });

    expect(result.takenAt).toBe(toRfc3339(takenAt));
    expect(result.takenAt).toMatch(/^2026-09-06T14:30:15[+-]\d{2}:\d{2}$/);
  });
});

describe("toRfc3339", () => {
  it("always carries an explicit offset the server can parse", () => {
    expect(toRfc3339(new Date(2026, 8, 6, 14, 0, 0))).toMatch(
      /^2026-09-06T14:00:00[+-]\d{2}:\d{2}$/,
    );
  });

  it("zero-pads every field", () => {
    expect(toRfc3339(new Date(2026, 0, 2, 3, 4, 5))).toMatch(
      /^2026-01-02T03:04:05[+-]\d{2}:\d{2}$/,
    );
  });

  it("formats a negative offset correctly", () => {
    const date = new Date(2026, 8, 6, 14, 0, 0);
    vi.spyOn(date, "getTimezoneOffset").mockReturnValue(300); // UTC-5
    expect(toRfc3339(date)).toBe("2026-09-06T14:00:00-05:00");
  });

  it("formats a half-hour offset correctly", () => {
    const date = new Date(2026, 8, 6, 14, 0, 0);
    vi.spyOn(date, "getTimezoneOffset").mockReturnValue(-330); // UTC+5:30
    expect(toRfc3339(date)).toBe("2026-09-06T14:00:00+05:30");
  });
});
