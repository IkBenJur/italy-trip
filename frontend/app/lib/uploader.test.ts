import "fake-indexeddb/auto";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Uploader, backoffMs, BASE_BACKOFF_MS, MAX_BACKOFF_MS } from "~/lib/uploader";
import * as queue from "~/lib/photoQueue";
import { ApiError } from "~/lib/apiClient";
import type { UploadResponse } from "~/types/event.types";

/**
 * enqueue deliberately does not await the upload, so tests wait for the drain it
 * kicks off.
 */
async function enqueueAndDrain(
  uploader: Uploader,
  item: { clientId: string; blob: Blob; takenAt: string },
) {
  await uploader.enqueue(item);
  await uploader.drain();
}

function capture(clientId: string, takenAt = "2026-09-06T14:00:00+02:00") {
  return {
    clientId,
    blob: new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10])], {
      type: "image/jpeg",
    }),
    takenAt,
  };
}

function ok(clientId: string, duplicate = false): UploadResponse {
  return { id: `server-${clientId}`, client_id: clientId, duplicate };
}

/** A controllable clock, so backoff can be observed without real waiting. */
function clock(start = 1_000_000) {
  let value = start;
  return {
    now: () => value,
    advance: (ms: number) => {
      value += ms;
    },
  };
}

beforeEach(async () => {
  queue.resetQueueConnection();
  await queue.clear();
});

afterEach(async () => {
  await queue.clear();
  queue.resetQueueConnection();
});

describe("backoffMs", () => {
  it("doubles from 2s and caps at 5 minutes", () => {
    expect(backoffMs(1)).toBe(2_000);
    expect(backoffMs(2)).toBe(4_000);
    expect(backoffMs(3)).toBe(8_000);
    expect(backoffMs(4)).toBe(16_000);
    expect(backoffMs(8)).toBe(256_000);
    expect(backoffMs(9)).toBe(MAX_BACKOFF_MS);
    expect(backoffMs(100)).toBe(MAX_BACKOFF_MS);
    expect(backoffMs(1)).toBe(BASE_BACKOFF_MS);
  });
});

describe("draining the queue", () => {
  it("enqueues, uploads, and empties the store", async () => {
    const upload = vi.fn(async (input: { clientId: string }) => ok(input.clientId));
    const uploader = new Uploader({ upload });

    await enqueueAndDrain(uploader, capture("a"));

    expect(upload).toHaveBeenCalledTimes(1);
    expect(upload.mock.calls[0]?.[0]).toMatchObject({
      clientId: "a",
      takenAt: "2026-09-06T14:00:00+02:00",
    });
    expect(await queue.all()).toHaveLength(0);
    expect(uploader.getStatus().pending).toBe(0);
  });

  it("uploads one at a time, oldest capture first", async () => {
    const order: string[] = [];
    let inFlight = 0;
    let maxInFlight = 0;

    const upload = vi.fn(async (input: { clientId: string }) => {
      inFlight += 1;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((resolve) => setTimeout(resolve, 0));
      order.push(input.clientId);
      inFlight -= 1;
      return ok(input.clientId);
    });

    const uploader = new Uploader({ upload });

    // Enqueued newest-first on purpose.
    await queue.enqueue(capture("third", "2026-09-08T10:00:00+02:00"));
    await queue.enqueue(capture("first", "2026-09-06T10:00:00+02:00"));
    await queue.enqueue(capture("second", "2026-09-07T10:00:00+02:00"));

    await uploader.drain();

    expect(order).toEqual(["first", "second", "third"]);
    expect(maxInFlight).toBe(1);
    expect(await queue.all()).toHaveLength(0);
  });
});

describe("retrying", () => {
  it("retries a network failure twice, then succeeds, uploading exactly once", async () => {
    const time = clock();
    let calls = 0;
    const upload = vi.fn(async () => {
      calls += 1;
      if (calls <= 2) throw new TypeError("Failed to fetch");
      return ok("a");
    });

    const uploader = new Uploader({ upload, now: time.now });
    await enqueueAndDrain(uploader, capture("a"));

    // First attempt failed; the item is still queued and backed off by 2s.
    let items = await queue.all();
    expect(items).toHaveLength(1);
    expect(items[0].attempts).toBe(1);
    expect(items[0].nextAttemptAt).toBe(time.now() + 2_000);
    expect(items[0].failed).toBeUndefined();

    // Not yet due: draining early must not consume an attempt.
    await uploader.drain();
    expect(upload).toHaveBeenCalledTimes(1);

    time.advance(2_000);
    await uploader.drain();
    expect(upload).toHaveBeenCalledTimes(2);
    items = await queue.all();
    expect(items[0].attempts).toBe(2);
    expect(items[0].nextAttemptAt).toBe(time.now() + 4_000);

    time.advance(4_000);
    await uploader.drain();

    // Three attempts total, and exactly one of them stuck.
    expect(upload).toHaveBeenCalledTimes(3);
    expect(await queue.all()).toHaveLength(0);
  });

  it("keeps retrying a 500", async () => {
    const time = clock();
    const upload = vi.fn(async () => {
      throw new ApiError(500, "internal server error");
    });

    const uploader = new Uploader({ upload, now: time.now });
    await enqueueAndDrain(uploader, capture("a"));

    for (let i = 1; i <= 3; i += 1) {
      time.advance(MAX_BACKOFF_MS);
      await uploader.drain();
    }

    const items = await queue.all();
    expect(items).toHaveLength(1);
    expect(items[0].failed).toBeUndefined();
    expect(items[0].attempts).toBe(4);
    expect(upload).toHaveBeenCalledTimes(4);
  });
});

describe("server responses that end the retry loop", () => {
  it("clears the item on a 200 duplicate rather than retrying forever", async () => {
    const upload = vi.fn(async () => ok("a", true));
    const uploader = new Uploader({ upload });

    await enqueueAndDrain(uploader, capture("a"));

    expect(upload).toHaveBeenCalledTimes(1);
    expect(await queue.all()).toHaveLength(0);
    expect(uploader.getStatus().pending).toBe(0);
  });

  it("drops the item on a 423 and says so", async () => {
    const upload = vi.fn(async () => {
      throw new ApiError(423, "the event is over; no more photos can be added");
    });
    const uploader = new Uploader({ upload });

    await enqueueAndDrain(uploader, capture("a"));

    // Gone, not retried: the server will never accept it.
    expect(await queue.all()).toHaveLength(0);
    expect(uploader.getStatus().message).toMatch(/ended before some photos/i);

    await uploader.drain();
    expect(upload).toHaveBeenCalledTimes(1);
  });

  it("marks the item failed on a 400 and stops retrying", async () => {
    const time = clock();
    const upload = vi.fn(async () => {
      throw new ApiError(400, "file must be a JPEG, detected text/plain");
    });
    const uploader = new Uploader({ upload, now: time.now });

    await enqueueAndDrain(uploader, capture("a"));

    const items = await queue.all();
    expect(items).toHaveLength(1);
    expect(items[0].failed).toBe(true);
    expect(items[0].lastError).toMatch(/must be a JPEG/);

    // No amount of time or draining tries it again.
    time.advance(MAX_BACKOFF_MS * 10);
    await uploader.drain();
    expect(upload).toHaveBeenCalledTimes(1);

    expect(uploader.getStatus()).toMatchObject({ pending: 0, failed: 1 });
  });

  it("does not retry a 401 either", async () => {
    const upload = vi.fn(async () => {
      throw new ApiError(401, "invalid or expired token");
    });
    const uploader = new Uploader({ upload });

    await enqueueAndDrain(uploader, capture("a"));
    await uploader.drain();

    expect(upload).toHaveBeenCalledTimes(1);
    expect((await queue.all())[0].failed).toBe(true);
  });
});

describe("surviving a reload", () => {
  it("keeps captures across a torn-down module, then uploads them", async () => {
    // Offline: every attempt fails.
    const offline = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    const first = new Uploader({ upload: offline });

    await enqueueAndDrain(first, capture("a", "2026-09-06T10:00:00+02:00"));
    await enqueueAndDrain(first, capture("b", "2026-09-06T11:00:00+02:00"));
    await enqueueAndDrain(first, capture("c", "2026-09-06T12:00:00+02:00"));

    expect(await queue.all()).toHaveLength(3);

    // The tab is closed and reopened: the connection is gone, the data is not.
    queue.resetQueueConnection();

    const survivors = await queue.all();
    expect(survivors.map((item) => item.clientId).sort()).toEqual(["a", "b", "c"]);
    expect(survivors.every((item) => item.bytes.byteLength > 0)).toBe(true);
    expect(survivors.every((item) => item.contentType === "image/jpeg")).toBe(true);
    // The bytes must still be the JPEG that was captured.
    expect(Array.from(new Uint8Array(survivors[0].bytes).slice(0, 3))).toEqual([0xff, 0xd8, 0xff]);

    // Back on signal.
    const online = vi.fn(async (input: { clientId: string }) => ok(input.clientId));
    const second = new Uploader({ upload: online, now: () => Date.now() + MAX_BACKOFF_MS });
    await second.drain();

    expect(online).toHaveBeenCalledTimes(3);
    expect(await queue.all()).toHaveLength(0);
  });

  it("preserves the exact capture time across the reload", async () => {
    await queue.enqueue(capture("a", "2026-09-06T14:30:15+02:00"));
    queue.resetQueueConnection();

    const upload = vi.fn(async (input: { clientId: string; takenAt: string }) =>
      ok(input.clientId),
    );
    await new Uploader({ upload }).drain();

    expect(upload.mock.calls[0]?.[0].takenAt).toBe("2026-09-06T14:30:15+02:00");
  });
});

describe("triggers", () => {
  it("drains when the browser comes back online", async () => {
    const time = clock();
    let online = false;
    const upload = vi.fn(async (input: { clientId: string }) => {
      if (!online) throw new TypeError("Failed to fetch");
      return ok(input.clientId);
    });

    const uploader = new Uploader({ upload, now: time.now });
    uploader.start();

    await enqueueAndDrain(uploader, capture("a"));
    expect(await queue.all()).toHaveLength(1);

    online = true;
    time.advance(MAX_BACKOFF_MS);
    window.dispatchEvent(new Event("online"));
    await vi.waitFor(async () => expect(await queue.all()).toHaveLength(0));

    uploader.stop();
  });

  it("drains when the tab becomes visible again", async () => {
    const time = clock();
    let online = false;
    const upload = vi.fn(async (input: { clientId: string }) => {
      if (!online) throw new TypeError("Failed to fetch");
      return ok(input.clientId);
    });

    const uploader = new Uploader({ upload, now: time.now });
    uploader.start();
    await enqueueAndDrain(uploader, capture("a"));

    online = true;
    time.advance(MAX_BACKOFF_MS);
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.waitFor(async () => expect(await queue.all()).toHaveLength(0));

    uploader.stop();
  });
});

describe("status", () => {
  it("reports the pending count to subscribers", async () => {
    const upload = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    const uploader = new Uploader({ upload });

    const seen: number[] = [];
    const unsubscribe = uploader.subscribe((status) => seen.push(status.pending));

    await enqueueAndDrain(uploader, capture("a"));
    await enqueueAndDrain(uploader, capture("b"));

    expect(uploader.getStatus().pending).toBe(2);
    expect(seen).toContain(2);

    unsubscribe();
  });
});
