import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Webcam from "react-webcam";
import { captureFrame, CaptureError, type CaptureResult } from "~/lib/capture";
import {
  DEFAULT_FACING,
  hasMultipleCameras,
  readFacing,
  writeFacing,
  type Facing,
} from "~/lib/cameraDevices";

/**
 * `navigator.mediaDevices` is only *defined* on https, localhost or 127.0.0.1.
 * Over plain http on a LAN IP it is `undefined` and touching it throws a
 * TypeError before any permission dialog appears. This is not a permission that
 * can be granted — the API is simply absent — so it needs its own message.
 */
function cameraApiAvailable(): boolean {
  return (
    typeof navigator !== "undefined" &&
    typeof navigator.mediaDevices !== "undefined" &&
    typeof navigator.mediaDevices.getUserMedia === "function"
  );
}

type Phase =
  | { kind: "checking" }
  | { kind: "unsupported" }
  | { kind: "denied" }
  | { kind: "error"; message: string }
  | { kind: "live" }
  | { kind: "review"; capture: CaptureResult; previewUrl: string };

export interface CameraViewProps {
  /** Called once the user confirms a capture. */
  onConfirm: (capture: CaptureResult) => void;
  /** Rendered over the viewfinder: countdown, counts, status. */
  overlay?: React.ReactNode;
}

export function CameraView({ onConfirm, overlay }: CameraViewProps) {
  const webcamRef = useRef<Webcam>(null);
  const [phase, setPhase] = useState<Phase>({ kind: "checking" });
  const [busy, setBusy] = useState(false);

  // Starts on the rear camera and is corrected from storage on mount: reading
  // localStorage during render would not match the prerendered HTML.
  const [facing, setFacing] = useState<Facing>(DEFAULT_FACING);

  // Only true once a live stream has proved there is a second camera.
  const [canFlip, setCanFlip] = useState(false);

  // Set while a flip is renegotiating the stream. The video is briefly 0x0 in
  // that window, which the shutter must not be pressed into.
  const [switching, setSwitching] = useState(false);

  // The camera to fall back to if the one being switched to refuses to start.
  const facingBeforeSwitch = useRef<Facing>(DEFAULT_FACING);

  // Bumping this remounts <Webcam>, which is how a dead stream is recovered.
  const [streamKey, setStreamKey] = useState(0);

  useEffect(() => {
    if (!cameraApiAvailable()) {
      setPhase({ kind: "unsupported" });
      return;
    }
    const stored = readFacing();
    setFacing(stored);
    facingBeforeSwitch.current = stored;
    setPhase({ kind: "live" });
  }, []);

  // The object URL for the review image is a real allocation. Leaking these on a
  // phone will eventually kill the tab, so every one is revoked.
  useEffect(() => {
    if (phase.kind !== "review") return;
    const url = phase.previewUrl;
    return () => URL.revokeObjectURL(url);
  }, [phase]);

  /**
   * iOS suspends the camera when the phone locks or the tab goes to the
   * background, and the track often comes back ended rather than resuming. On
   * return, if the track is not live, remount the component to ask for a fresh
   * one.
   */
  useEffect(() => {
    function onVisible() {
      if (document.visibilityState !== "visible") return;

      const stream = webcamRef.current?.stream;
      const track = stream?.getVideoTracks()[0];
      if (!track || track.readyState === "ended") {
        setStreamKey((key) => key + 1);
      }
    }

    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", onVisible);
    return () => {
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", onVisible);
    };
  }, []);

  /**
   * A live stream is the first moment enumerateDevices() can be trusted, so the
   * "is there a second camera" probe hangs off it rather than off mount.
   */
  const handleUserMedia = useCallback(() => {
    setSwitching(false);
    if (!canFlip) {
      void hasMultipleCameras().then(setCanFlip);
    }
  }, [canFlip]);

  const handleUserMediaError = useCallback(
    (error: string | DOMException) => {
      // A camera that refuses to start mid-flip should hand back the one that
      // was already working, not take over the screen with an error.
      if (switching) {
        const previous = facingBeforeSwitch.current;
        setSwitching(false);
        setFacing(previous);
        writeFacing(previous);
        return;
      }

      const name = typeof error === "string" ? error : error.name;
      if (name === "NotAllowedError" || name === "PermissionDeniedError" || name === "SecurityError") {
        setPhase({ kind: "denied" });
        return;
      }
      setPhase({
        kind: "error",
        message: typeof error === "string" ? error : `${error.name}: ${error.message}`,
      });
    },
    [switching],
  );

  const flip = useCallback(() => {
    if (switching || busy) return;

    const next: Facing = facing === "user" ? "environment" : "user";
    facingBeforeSwitch.current = facing;
    writeFacing(next);
    setFacing(next);
    setSwitching(true);
  }, [busy, facing, switching]);

  // react-webcam JSON-compares this prop and renegotiates the stream when it
  // changes, which is the whole switching mechanism.
  const videoConstraints = useMemo(
    () => ({
      facingMode: { ideal: facing },
      width: { ideal: 1920 },
      height: { ideal: 1080 },
    }),
    [facing],
  );

  const shoot = useCallback(async () => {
    const video = webcamRef.current?.video;
    if (!video || busy) return;

    setBusy(true);
    try {
      const capture = await captureFrame(video);
      setPhase({ kind: "review", capture, previewUrl: URL.createObjectURL(capture.blob) });
    } catch (error) {
      const message =
        error instanceof CaptureError ? error.message : "could not take the photo";
      setPhase({ kind: "error", message });
    } finally {
      setBusy(false);
    }
  }, [busy]);

  const retake = useCallback(() => {
    setPhase({ kind: "live" });
  }, []);

  const confirm = useCallback(() => {
    if (phase.kind !== "review") return;
    onConfirm(phase.capture);
    setPhase({ kind: "live" });
  }, [phase, onConfirm]);

  if (phase.kind === "checking") {
    return <Message title="Starting the camera…" />;
  }

  if (phase.kind === "unsupported") {
    return (
      <Message title="This page needs HTTPS">
        <p>
          Browsers only hand out the camera on a secure connection. Open the app on its{" "}
          <span className="font-medium">https://</span> address rather than an IP address on
          the local network.
        </p>
      </Message>
    );
  }

  if (phase.kind === "denied") {
    return (
      <Message title="Camera access is blocked">
        <p>Safari is refusing the camera for this site. To re-enable it:</p>
        <ol className="mt-3 list-decimal space-y-1 pl-5 text-left">
          <li>Tap the {"\u{1F6DF}"} / “AA” button in the address bar</li>
          <li>Choose <span className="font-medium">Website Settings</span></li>
          <li>Set <span className="font-medium">Camera</span> to <span className="font-medium">Allow</span></li>
          <li>Reload this page</li>
        </ol>
        <button
          type="button"
          onClick={() => {
            setStreamKey((key) => key + 1);
            setPhase({ kind: "live" });
          }}
          className="mt-5 rounded-full bg-white px-5 py-2 font-medium text-black"
        >
          Try again
        </button>
      </Message>
    );
  }

  if (phase.kind === "error") {
    return (
      <Message title="The camera would not start">
        <p className="wrap-break-word">{phase.message}</p>
        <button
          type="button"
          onClick={() => {
            setStreamKey((key) => key + 1);
            setPhase({ kind: "live" });
          }}
          className="mt-5 rounded-full bg-white px-5 py-2 font-medium text-black"
        >
          Try again
        </button>
      </Message>
    );
  }

  const reviewing = phase.kind === "review";

  return (
    <div className="relative min-h-dvh bg-black">
      {/* The viewfinder stays mounted under the review overlay so returning to
          it does not have to renegotiate the stream. */}
      <Webcam
        key={streamKey}
        ref={webcamRef}
        audio={false}
        playsInline
        // CSS-only, so the selfie viewfinder reads like a mirror while
        // captureFrame — which draws the raw video — still saves the true image.
        mirrored={facing === "user"}
        screenshotFormat="image/jpeg"
        onUserMedia={handleUserMedia}
        onUserMediaError={handleUserMediaError}
        videoConstraints={videoConstraints}
        className="h-dvh w-full object-cover"
      />

      {reviewing && (
        <img
          src={phase.previewUrl}
          alt="The photo you just took"
          className="absolute inset-0 h-dvh w-full object-cover"
        />
      )}

      {!reviewing && overlay && (
        <div className="pointer-events-none absolute inset-x-0 top-0 p-4">{overlay}</div>
      )}

      <div className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-6 bg-linear-to-t from-black/70 to-transparent px-6 pb-10 pt-16">
        {reviewing ? (
          <>
            <button
              type="button"
              onClick={retake}
              className="rounded-full border border-white/60 px-6 py-3 font-medium text-white"
            >
              Retake
            </button>
            <button
              type="button"
              onClick={confirm}
              className="rounded-full bg-white px-8 py-3 font-semibold text-black"
            >
              Keep it
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={shoot}
              disabled={busy || switching}
              aria-label="Take a photo"
              className="h-20 w-20 rounded-full border-4 border-white/70 bg-white/95 transition active:scale-95 disabled:opacity-50"
            />

            {/* Absolutely placed so that adding it does not shove the shutter
                off the centre of the screen. */}
            {canFlip && (
              <button
                type="button"
                onClick={flip}
                disabled={switching}
                aria-label={
                  facing === "user" ? "Switch to the rear camera" : "Switch to the selfie camera"
                }
                className="absolute right-8 flex h-12 w-12 items-center justify-center rounded-full bg-white/20 text-white backdrop-blur-sm transition active:scale-95 disabled:opacity-40"
              >
                <FlipIcon />
              </button>
            )}
          </>
        )}
      </div>
    </div>
  );
}

/** Two arrows chasing each other round a circle: "turn the camera around". */
function FlipIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className="h-6 w-6"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  );
}

function Message({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-black p-8 text-white">
      <div className="max-w-sm text-center">
        <h1 className="text-lg font-semibold">{title}</h1>
        <div className="mt-3 text-sm leading-relaxed opacity-80">{children}</div>
      </div>
    </main>
  );
}
