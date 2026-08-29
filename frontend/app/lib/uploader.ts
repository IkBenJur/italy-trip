/**
 * Drains the capture queue in the background.
 *
 * One at a time, oldest capture first, with exponential backoff on anything
 * that might succeed later and an immediate stop on anything that never will.
 */
import { ApiError } from "~/lib/apiClient";
import { photoService } from "~/services/photo.service";
import * as queue from "~/lib/photoQueue";
import type { PendingPhoto } from "~/lib/photoQueue";

/** First retry after 2s, then 4s, 8s, … never longer than 5 minutes. */
export const BASE_BACKOFF_MS = 2_000;
export const MAX_BACKOFF_MS = 5 * 60_000;
/** A sweep every minute catches anything the event triggers missed. */
export const SWEEP_INTERVAL_MS = 60_000;

export function backoffMs(attempts: number): number {
  return Math.min(BASE_BACKOFF_MS * 2 ** Math.max(0, attempts - 1), MAX_BACKOFF_MS);
}

export interface UploaderStatus {
  pending: number;
  failed: number;
  /** Set when a capture was dropped because the event ended while it waited. */
  message: string | null;
}

type Listener = (status: UploaderStatus) => void;

export interface UploaderOptions {
  upload?: typeof photoService.upload;
  now?: () => number;
}

export class Uploader {
  private readonly upload: typeof photoService.upload;
  private readonly now: () => number;

  private listeners = new Set<Listener>();
  private status: UploaderStatus = { pending: 0, failed: 0, message: null };

  /** The in-flight drain, if any. Callers await this rather than starting a second. */
  private draining: Promise<void> | null = null;
  /** Set when a drain is requested while one is already running. */
  private again = false;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private sweepTimer: ReturnType<typeof setInterval> | null = null;
  private started = false;

  constructor(options: UploaderOptions = {}) {
    this.upload = options.upload ?? photoService.upload;
    this.now = options.now ?? (() => Date.now());
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    listener(this.status);
    return () => this.listeners.delete(listener);
  }

  getStatus(): UploaderStatus {
    return this.status;
  }

  /** Wires up the triggers: reconnecting, returning to the tab, and a sweep. */
  start(): void {
    if (this.started) return;
    this.started = true;

    if (typeof window !== "undefined") {
      window.addEventListener("online", this.onTrigger);
      document.addEventListener("visibilitychange", this.onVisibility);
      this.sweepTimer = setInterval(this.onTrigger, SWEEP_INTERVAL_MS);
    }

    void this.drain();
  }

  stop(): void {
    if (!this.started) return;
    this.started = false;

    if (typeof window !== "undefined") {
      window.removeEventListener("online", this.onTrigger);
      document.removeEventListener("visibilitychange", this.onVisibility);
    }
    if (this.sweepTimer) clearInterval(this.sweepTimer);
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.sweepTimer = null;
    this.retryTimer = null;
  }

  private onTrigger = () => {
    void this.drain();
  };

  private onVisibility = () => {
    if (document.visibilityState === "visible") void this.drain();
  };

  /** Writes a capture to durable storage, then tries to send it. */
  async enqueue(item: { clientId: string; blob: Blob; takenAt: string }): Promise<void> {
    await queue.enqueue(item);
    await this.refreshCounts();
    // Not awaited: the shutter must never wait on the network.
    void this.drain();
  }

  /**
   * Sends every due item, one at a time. Concurrency is deliberately 1: these
   * are multi-hundred-KB uploads on a phone with one bar of signal.
   */
  async drain(): Promise<void> {
    if (this.draining) {
      // Fold this request into the running drain rather than racing it, and
      // still return a promise that resolves when the work is actually done.
      this.again = true;
      return this.draining;
    }

    this.draining = this.runDrain();
    try {
      await this.draining;
    } finally {
      this.draining = null;
    }
  }

  private async runDrain(): Promise<void> {
    try {
      do {
        this.again = false;
        for (const item of await queue.due(this.now())) {
          await this.send(item);
        }
      } while (this.again);
    } finally {
      await this.refreshCounts();
      await this.scheduleNextRetry();
    }
  }

  private async send(item: PendingPhoto): Promise<void> {
    try {
      // 201 (stored) and 200 (the server already had it) are both success: the
      // capture is safely on the server either way.
      await this.upload({
        blob: queue.toBlob(item),
        clientId: item.clientId,
        takenAt: item.takenAt,
      });
      await queue.remove(item.clientId);
      return;
    } catch (error) {
      await this.handleFailure(item, error);
    }
  }

  private async handleFailure(item: PendingPhoto, error: unknown): Promise<void> {
    const attempts = item.attempts + 1;
    const message = error instanceof Error ? error.message : String(error);

    if (error instanceof ApiError && error.isLocked) {
      // The event ended while this sat in the queue. The server will never take
      // it, so keeping it would mean retrying forever.
      await queue.remove(item.clientId);
      this.setStatus({
        ...this.status,
        message: "The trip ended before some photos finished uploading. Those are lost.",
      });
      return;
    }

    if (error instanceof ApiError && error.isPermanent) {
      // A 400 or 401 will not become a 201 on the tenth try.
      await queue.put({ ...item, attempts, failed: true, lastError: message });
      return;
    }

    // 5xx, a network error, a dead tunnel: try again later.
    await queue.put({
      ...item,
      attempts,
      nextAttemptAt: this.now() + backoffMs(attempts),
      lastError: message,
    });
  }

  /** Wakes the uploader exactly when the earliest backoff expires. */
  private async scheduleNextRetry(): Promise<void> {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    if (!this.started) return;

    const waiting = (await queue.all()).filter((item) => !item.failed);
    if (waiting.length === 0) return;

    const soonest = Math.min(...waiting.map((item) => item.nextAttemptAt));
    const delay = Math.max(0, soonest - this.now());

    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      void this.drain();
    }, delay);
  }

  private async refreshCounts(): Promise<void> {
    const { pending, failed } = await queue.counts();
    this.setStatus({ ...this.status, pending, failed });
  }

  private setStatus(status: UploaderStatus): void {
    this.status = status;
    for (const listener of this.listeners) listener(status);
  }

  /** Clears the "photos were lost" notice once it has been read. */
  dismissMessage(): void {
    this.setStatus({ ...this.status, message: null });
  }
}

/** The app's single uploader. */
export const uploader = new Uploader();
