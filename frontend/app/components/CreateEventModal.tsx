import { useEffect, useState, type FormEvent } from "react";
import { Button } from "~/components/ui/Button";
import { useCreateEvent } from "~/hooks/useCreateEvent";
import type { EventInfo } from "~/types/event.types";

interface CreateEventModalProps {
  open: boolean;
  onClose: () => void;
  onCreated?: (event: EventInfo) => void;
}

/**
 * datetime-local inputs are parsed by `new Date()` as local time, and
 * `.toISOString()` turns that into RFC3339 (UTC, "Z" suffix) for the API —
 * so no manual timezone math is needed here.
 */
function toIso(datetimeLocal: string): string | null {
  if (!datetimeLocal) return null;
  const date = new Date(datetimeLocal);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

export function CreateEventModal({ open, onClose, onCreated }: CreateEventModalProps) {
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const createEvent = useCreateEvent();

  useEffect(() => {
    if (open) {
      setStartsAt("");
      setEndsAt("");
      setValidationError(null);
      createEvent.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  if (!open) return null;

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setValidationError(null);

    const startsIso = toIso(startsAt);
    const endsIso = toIso(endsAt);
    if (!startsIso || !endsIso) {
      setValidationError("Pick a start and end date and time.");
      return;
    }
    if (endsIso <= startsIso) {
      setValidationError("End must be after start.");
      return;
    }

    createEvent.mutate(
      { starts_at: startsIso, ends_at: endsIso },
      {
        onSuccess: (created) => {
          onCreated?.(created);
          onClose();
        },
      },
    );
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onClick={() => !createEvent.isPending && onClose()}
    >
      <div
        className="w-full max-w-sm rounded-xl border border-white/10 bg-neutral-900 p-6 text-white"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold">New event</h2>
        <p className="mt-1 text-sm opacity-60">
          Past events and their photos stay in the album under their own event.
        </p>

        <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-4">
          <label className="flex flex-col gap-1 text-sm">
            Starts
            <input
              type="datetime-local"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
              className="rounded-md border border-white/20 bg-neutral-950 px-3 py-2 text-white"
              required
            />
          </label>
          <label className="flex flex-col gap-1 text-sm">
            Ends
            <input
              type="datetime-local"
              value={endsAt}
              onChange={(e) => setEndsAt(e.target.value)}
              className="rounded-md border border-white/20 bg-neutral-950 px-3 py-2 text-white"
              required
            />
          </label>

          {(validationError || createEvent.error) && (
            <p className="text-sm text-red-400">
              {validationError ?? createEvent.error?.message}
            </p>
          )}

          <div className="mt-2 flex justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={onClose}
              disabled={createEvent.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={createEvent.isPending}>
              {createEvent.isPending ? "Starting…" : "Start event"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
