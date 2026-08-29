/**
 * The durable capture queue.
 *
 * Every photo is written to IndexedDB before anything is sent. That is what
 * makes the camera usable in a tunnel, a dead zone, or with the tab closed: the
 * shutter always succeeds, and the upload is a separate problem solved later.
 *
 * The key is the clientId, which is generated once per capture and reused on
 * every retry. The server has UNIQUE (event_id, client_id), so a request that
 * succeeded but whose response was lost cannot produce a duplicate photo.
 */
import { openDB, type IDBPDatabase } from "idb";

export const DB_NAME = "italy-trip";
export const DB_VERSION = 1;
export const STORE = "pending";

export interface PendingPhoto {
  clientId: string;
  /**
   * The JPEG as raw bytes rather than a Blob. Structured clone handles
   * ArrayBuffer everywhere, whereas storing a Blob in IndexedDB has a history of
   * going wrong in Safari — which is the only browser this app has to work in.
   */
  bytes: ArrayBuffer;
  contentType: string;
  /** RFC3339 with offset, recorded at capture time. */
  takenAt: string;
  /** How many upload attempts have been made. */
  attempts: number;
  /** Epoch ms; the item is not retried before this. */
  nextAttemptAt: number;
  /** Set when the server rejected the photo in a way retrying cannot fix. */
  failed?: boolean;
  lastError?: string;
}

let dbPromise: Promise<IDBPDatabase> | null = null;

function db(): Promise<IDBPDatabase> {
  if (!dbPromise) {
    dbPromise = openDB(DB_NAME, DB_VERSION, {
      upgrade(database) {
        if (!database.objectStoreNames.contains(STORE)) {
          database.createObjectStore(STORE, { keyPath: "clientId" });
        }
      },
    });
  }
  return dbPromise;
}

/**
 * Drops the cached connection. Used by tests to simulate a page reload; the
 * data in IndexedDB is untouched.
 */
export function resetQueueConnection() {
  dbPromise = null;
}

export async function enqueue(item: {
  clientId: string;
  blob: Blob;
  takenAt: string;
}): Promise<PendingPhoto> {
  const pending: PendingPhoto = {
    clientId: item.clientId,
    bytes: await item.blob.arrayBuffer(),
    contentType: item.blob.type || "image/jpeg",
    takenAt: item.takenAt,
    attempts: 0,
    nextAttemptAt: 0,
  };
  await (await db()).put(STORE, pending);
  return pending;
}

/** Rebuilds the uploadable Blob from a stored item. */
export function toBlob(item: PendingPhoto): Blob {
  return new Blob([item.bytes], { type: item.contentType });
}

export async function all(): Promise<PendingPhoto[]> {
  return (await db()).getAll(STORE) as Promise<PendingPhoto[]>;
}

/** Items still worth trying: not permanently failed, and past their backoff. */
export async function due(now: number): Promise<PendingPhoto[]> {
  const items = await all();
  return items
    .filter((item) => !item.failed && item.nextAttemptAt <= now)
    .sort((a, b) => a.takenAt.localeCompare(b.takenAt));
}

export async function put(item: PendingPhoto): Promise<void> {
  await (await db()).put(STORE, item);
}

export async function remove(clientId: string): Promise<void> {
  await (await db()).delete(STORE, clientId);
}

export async function clear(): Promise<void> {
  await (await db()).clear(STORE);
}

/** Counts for the camera screen: still trying, versus given up on. */
export async function counts(): Promise<{ pending: number; failed: number }> {
  const items = await all();
  return {
    pending: items.filter((item) => !item.failed).length,
    failed: items.filter((item) => item.failed).length,
  };
}
