import { useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { useEvent } from "~/hooks/useEvent";
import { getToken } from "~/lib/auth";
import { ApiError } from "~/lib/apiClient";
import { CreateEventModal } from "~/components/CreateEventModal";
import { Button } from "~/components/ui/Button";
import type { EventInfo } from "~/types/event.types";

/**
 * The lock gate.
 *
 * It routes off `event.is_over`, which the server computes, and never off a
 * local `Date.now()` comparison against `ends_at`. A phone with a wrong clock,
 * or a devtools console, must not be able to reach the album early — and since
 * the API refuses anyway, this gate is about showing the right screen rather
 * than about enforcement.
 */
export default function Gate() {
  const location = useLocation();
  const { data: event, isPending, error } = useEvent();
  const [showCreateEvent, setShowCreateEvent] = useState(false);

  if (!getToken()) {
    return <Navigate to="/login" replace />;
  }

  if (isPending) {
    return <Splash>Loading…</Splash>;
  }

  if (error) {
    if (error instanceof ApiError && error.isUnauthorized) {
      return <Navigate to="/login" replace />;
    }
    if (error instanceof ApiError && error.isNotFound) {
      return (
        <>
          <Splash>
            <p className="font-medium">No event has been started yet.</p>
            <Button className="mt-4" onClick={() => setShowCreateEvent(true)}>
              Start an event
            </Button>
          </Splash>
          <CreateEventModal open={showCreateEvent} onClose={() => setShowCreateEvent(false)} />
        </>
      );
    }
    return (
      <Splash>
        <p className="font-medium">Could not reach the server.</p>
        <p className="mt-2 text-sm opacity-70">{error.message}</p>
        <p className="mt-4 text-sm opacity-70">It will keep trying.</p>
      </Splash>
    );
  }

  const onCamera = location.pathname === "/";

  // While the trip runs the app is only a camera; every album route bounces.
  if (!event.is_over && !onCamera) {
    return <Navigate to="/" replace />;
  }

  // Once it is over the camera is gone and the album takes its place.
  if (event.is_over && onCamera) {
    return <Navigate to="/album" replace />;
  }

  return <Outlet context={event satisfies EventInfo} />;
}

function Splash({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-black p-8 text-center text-white">
      <div>{children}</div>
    </main>
  );
}
