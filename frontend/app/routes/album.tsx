import { useState } from "react";
import { Link } from "react-router";
import { usePhotos } from "~/hooks/usePhotos";
import { useGateEvent } from "~/hooks/useGateEvent";
import { downloadAll } from "~/lib/download";

export function meta() {
  return [{ title: "Italy Trip — Album" }];
}

export default function Album() {
  const event = useGateEvent();
  const { data, isPending, error } = usePhotos(event.is_over);
  const [progress, setProgress] = useState<string | null>(null);

  const photos = data?.photos ?? [];

  return (
    <main className="min-h-dvh bg-neutral-950 text-white">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-4 border-b border-white/10 bg-neutral-950/85 px-4 py-3 backdrop-blur">
        <div>
          <h1 className="text-base font-semibold">{event.name}</h1>
          <p className="text-xs opacity-60">
            {photos.length} {photos.length === 1 ? "photo" : "photos"}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {photos.length > 0 && (
            <>
              <Link
                to="/slideshow"
                className="rounded-full bg-white px-4 py-2 text-sm font-medium text-black"
              >
                Slideshow
              </Link>
              <button
                type="button"
                onClick={async () => {
                  setProgress("Starting…");
                  try {
                    await downloadAll(photos, (done, total) =>
                      setProgress(`${done} / ${total}`),
                    );
                    setProgress("Done");
                  } catch (downloadError) {
                    setProgress(
                      downloadError instanceof Error ? downloadError.message : "Failed",
                    );
                  }
                }}
                className="rounded-full border border-white/30 px-4 py-2 text-sm"
              >
                {progress ?? "Download all"}
              </button>
            </>
          )}
        </div>
      </header>

      {isPending && <p className="p-6 text-sm opacity-60">Loading the album…</p>}

      {error && (
        <p className="p-6 text-sm text-red-400">
          Could not load the album: {error.message}
        </p>
      )}

      {!isPending && !error && photos.length === 0 && (
        <p className="p-6 text-sm opacity-60">No photos were taken.</p>
      )}

      <div className="grid grid-cols-2 gap-1 p-1 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
        {photos.map((photo) => (
          <Link
            key={photo.id}
            to={`/album/${photo.id}`}
            // The aspect ratio comes from the stored dimensions, so tiles hold
            // their shape and the grid does not jump as images arrive.
            style={{ aspectRatio: `${photo.width} / ${photo.height}` }}
            className="block overflow-hidden bg-neutral-900"
          >
            <img
              src={photo.thumb_url}
              alt={`Taken ${new Date(photo.taken_at).toLocaleString()}`}
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover"
            />
          </Link>
        ))}
      </div>
    </main>
  );
}
