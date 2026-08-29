import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { usePhotos } from "~/hooks/usePhotos";
import { useGateEvent } from "~/hooks/useGateEvent";
import { whenLoaded } from "~/lib/preload";
import { releaseWakeLock, requestWakeLock } from "~/lib/wakeLock";

export function meta() {
  return [{ title: "Italy Trip — Slideshow" }];
}

export const SLIDE_MS = 4_000;

export default function Slideshow() {
  const event = useGateEvent();
  const { data, isPending } = usePhotos(event.is_over);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [chromeVisible, setChromeVisible] = useState(true);

  const photos = data?.photos ?? [];
  const total = photos.length;

  const advance = useCallback(
    (step: number) => {
      if (total === 0) return;
      // Wraps in both directions, so it runs unattended all evening.
      setIndex((current) => (current + step + total) % total);
    },
    [total],
  );

  /**
   * Waits for the next image to be decodable before swapping to it. Without
   * this, each advance flashes white while the JPEG downloads.
   */
  useEffect(() => {
    if (!playing || total <= 1) return;

    let cancelled = false;
    const nextUrl = photos[(index + 1) % total]?.url;

    const timer = setTimeout(async () => {
      await whenLoaded(nextUrl);
      if (!cancelled) advance(1);
    }, SLIDE_MS);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [playing, index, total, photos, advance]);

  // Keep the screen on while playing, where the browser allows it. Safari's
  // support is patchy, so this is best-effort and never surfaces an error.
  const sentinel = useRef<Awaited<ReturnType<typeof requestWakeLock>>>(null);

  useEffect(() => {
    let cancelled = false;

    async function acquire() {
      if (!playing) {
        await releaseWakeLock(sentinel.current);
        sentinel.current = null;
        return;
      }
      const lock = await requestWakeLock();
      if (cancelled) {
        await releaseWakeLock(lock);
        return;
      }
      sentinel.current = lock;
    }

    void acquire();

    // The lock is dropped whenever the tab is backgrounded; take it again on return.
    function onVisible() {
      if (document.visibilityState === "visible" && playing) void acquire();
    }
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", onVisible);
      void releaseWakeLock(sentinel.current);
      sentinel.current = null;
    };
  }, [playing]);

  useEffect(() => {
    function onKey(keyEvent: KeyboardEvent) {
      if (keyEvent.key === " ") {
        keyEvent.preventDefault();
        setPlaying((value) => !value);
      }
      if (keyEvent.key === "ArrowRight") advance(1);
      if (keyEvent.key === "ArrowLeft") advance(-1);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [advance]);

  if (isPending) return <Shell>Loading…</Shell>;

  if (total === 0) {
    return (
      <Shell>
        <p>There is nothing to show yet.</p>
        <Link to="/album" className="mt-4 inline-block underline">
          Back to the album
        </Link>
      </Shell>
    );
  }

  const photo = photos[index]!;

  return (
    <main
      className="relative min-h-dvh bg-black text-white"
      onClick={() => setChromeVisible((visible) => !visible)}
    >
      <img
        key={photo.id}
        src={photo.url}
        alt={`Taken ${new Date(photo.taken_at).toLocaleString()}`}
        className="h-dvh w-full object-contain"
      />

      {chromeVisible && (
        <>
          <header className="absolute inset-x-0 top-0 flex items-center justify-between bg-gradient-to-b from-black/70 to-transparent px-4 py-3 text-sm">
            <Link
              to="/album"
              onClick={(clickEvent) => clickEvent.stopPropagation()}
              className="rounded-full bg-black/50 px-4 py-2"
            >
              Back
            </Link>
            <span className="opacity-70 tabular-nums">
              {index + 1} / {total}
            </span>
          </header>

          <footer
            className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-4 bg-gradient-to-t from-black/70 to-transparent px-4 py-6"
            onClick={(clickEvent) => clickEvent.stopPropagation()}
          >
            <button
              type="button"
              onClick={() => advance(-1)}
              aria-label="Previous photo"
              className="rounded-full bg-black/50 px-5 py-3"
            >
              ‹
            </button>
            <button
              type="button"
              onClick={() => setPlaying((value) => !value)}
              className="rounded-full bg-white px-6 py-3 font-medium text-black"
            >
              {playing ? "Pause" : "Play"}
            </button>
            <button
              type="button"
              onClick={() => advance(1)}
              aria-label="Next photo"
              className="rounded-full bg-black/50 px-5 py-3"
            >
              ›
            </button>
          </footer>
        </>
      )}
    </main>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-black p-8 text-center text-white">
      <div>{children}</div>
    </main>
  );
}
