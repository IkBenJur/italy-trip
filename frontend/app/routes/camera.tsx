import { useCallback, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CameraView } from "~/components/CameraView";
import { Countdown } from "~/components/Countdown";
import { useGateEvent } from "~/hooks/useGateEvent";
import { useUploader } from "~/hooks/useUploader";
import { eventQueryKey } from "~/hooks/useEvent";
import { uploader } from "~/lib/uploader";
import type { CaptureResult } from "~/lib/capture";

export function meta() {
  return [{ title: "Italy Trip" }];
}

export default function Camera() {
  const event = useGateEvent();
  const queryClient = useQueryClient();
  const { pending, failed, message, dismissMessage } = useUploader();
  const previousPending = useRef(pending);

  // Once the queue drains, the server's photo_count has moved on.
  useEffect(() => {
    if (previousPending.current > 0 && pending === 0) {
      queryClient.invalidateQueries({ queryKey: eventQueryKey });
    }
    previousPending.current = pending;
  }, [pending, queryClient]);

  /**
   * The capture is written to IndexedDB and the shutter is free again
   * immediately. Whether there is any signal right now is not this function's
   * problem.
   */
  const onConfirm = useCallback((capture: CaptureResult) => {
    void uploader.enqueue({
      clientId: crypto.randomUUID(),
      blob: capture.blob,
      takenAt: capture.takenAt,
    });
  }, []);

  return (
    <CameraView
      onConfirm={onConfirm}
      overlay={
        <div className="space-y-2">
          <div className="flex items-start justify-between rounded-2xl bg-black/45 px-4 py-3 text-white backdrop-blur-sm">
            <div>
              <p className="text-xs uppercase tracking-wide opacity-70">Album opens in</p>
              <p className="text-base font-semibold tabular-nums">
                <Countdown endsAt={event.ends_at} />
              </p>
            </div>
            <div className="text-right">
              <p className="text-xs uppercase tracking-wide opacity-70">Photos</p>
              <p className="text-base font-semibold tabular-nums">{event.photo_count}</p>
            </div>
          </div>

          {pending > 0 && (
            <p className="rounded-full bg-black/45 px-4 py-2 text-center text-xs text-white backdrop-blur-sm">
              {pending} waiting to upload
            </p>
          )}

          {failed > 0 && (
            <p className="rounded-full bg-red-900/70 px-4 py-2 text-center text-xs text-white backdrop-blur-sm">
              {failed} could not be uploaded
            </p>
          )}

          {message && (
            <button
              type="button"
              onClick={dismissMessage}
              className="pointer-events-auto w-full rounded-2xl bg-red-900/80 px-4 py-3 text-left text-xs text-white backdrop-blur-sm"
            >
              {message} <span className="underline">Dismiss</span>
            </button>
          )}
        </div>
      }
    />
  );
}
