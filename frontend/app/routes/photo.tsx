import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { usePhotos } from "~/hooks/usePhotos";
import { useGateEvent } from "~/hooks/useGateEvent";
import { downloadPhoto } from "~/lib/download";
import { preloadImage } from "~/lib/preload";

export function meta() {
  return [{ title: "Italy Trip — Photo" }];
}

/** Below this many pixels a horizontal drag is a tap, not a swipe. */
const SWIPE_THRESHOLD = 50;

export default function PhotoViewer() {
  const { photoId } = useParams();
  const event = useGateEvent();
  const navigate = useNavigate();
  const { data, isPending } = usePhotos(event.is_over);
  const [status, setStatus] = useState<string | null>(null);

  const photos = data?.photos ?? [];
  const index = photos.findIndex((photo) => photo.id === photoId);
  const photo = index >= 0 ? photos[index] : undefined;

  const previous = index > 0 ? photos[index - 1] : undefined;
  const next = index >= 0 && index < photos.length - 1 ? photos[index + 1] : undefined;

  // The viewer is a real route, so the phone's back gesture closes it and each
  // photo is linkable. Navigation between photos replaces the entry so back
  // returns to the grid rather than walking every photo in reverse.
  const go = useCallback(
    (target: typeof photo) => {
      if (target) navigate(`/album/${target.id}`, { replace: true });
    },
    [navigate],
  );

  useEffect(() => {
    preloadImage(previous?.url);
    preloadImage(next?.url);
  }, [previous?.url, next?.url]);

  useEffect(() => {
    function onKey(keyEvent: KeyboardEvent) {
      if (keyEvent.key === "ArrowLeft") go(previous);
      if (keyEvent.key === "ArrowRight") go(next);
      if (keyEvent.key === "Escape") navigate("/album");
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [go, previous, next, navigate]);

  const touchStart = useRef<number | null>(null);

  if (isPending) {
    return <Shell>Loading…</Shell>;
  }

  if (!photo) {
    return (
      <Shell>
        <p>That photo is not in the album.</p>
        <Link to="/album" className="mt-4 inline-block underline">
          Back to the album
        </Link>
      </Shell>
    );
  }

  return (
    <main
      className="relative min-h-dvh touch-pan-y bg-black text-white"
      onTouchStart={(touchEvent) => {
        touchStart.current = touchEvent.changedTouches[0]?.clientX ?? null;
      }}
      onTouchEnd={(touchEvent) => {
        const start = touchStart.current;
        touchStart.current = null;
        if (start === null) return;

        const delta = (touchEvent.changedTouches[0]?.clientX ?? start) - start;
        if (delta <= -SWIPE_THRESHOLD) go(next);
        if (delta >= SWIPE_THRESHOLD) go(previous);
      }}
    >
      <img
        key={photo.id}
        src={photo.url}
        alt={`Taken ${new Date(photo.taken_at).toLocaleString()}`}
        className="h-dvh w-full object-contain"
      />

      <header className="absolute inset-x-0 top-0 flex items-center justify-between bg-gradient-to-b from-black/70 to-transparent px-4 py-3 text-sm">
        <Link to="/album" className="rounded-full bg-black/50 px-4 py-2">
          Back
        </Link>
        <span className="opacity-70 tabular-nums">
          {index + 1} / {photos.length}
        </span>
      </header>

      <footer className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 bg-gradient-to-t from-black/70 to-transparent px-4 py-6 text-sm">
        <button
          type="button"
          onClick={() => go(previous)}
          disabled={!previous}
          aria-label="Previous photo"
          className="rounded-full bg-black/50 px-5 py-3 disabled:opacity-30"
        >
          ‹
        </button>

        <button
          type="button"
          onClick={async () => {
            setStatus("Saving…");
            try {
              await downloadPhoto(photo);
              setStatus("Saved");
            } catch (downloadError) {
              setStatus(downloadError instanceof Error ? downloadError.message : "Failed");
            }
          }}
          className="rounded-full bg-white px-5 py-3 font-medium text-black"
        >
          {status ?? "Download"}
        </button>

        <button
          type="button"
          onClick={() => go(next)}
          disabled={!next}
          aria-label="Next photo"
          className="rounded-full bg-black/50 px-5 py-3 disabled:opacity-30"
        >
          ›
        </button>
      </footer>
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
