import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import Gate from "~/routes/gate";
import { eventService } from "~/services/event.service";
import { ApiError } from "~/lib/apiClient";
import type { EventInfo } from "~/types/event.types";

function eventInfo(overrides: Partial<EventInfo> = {}): EventInfo {
  return {
    id: "event-1",
    name: "Italy Trip",
    starts_at: "2026-09-05T00:00:00+02:00",
    ends_at: "2026-09-14T23:59:59+02:00",
    is_over: false,
    photo_count: 7,
    ...overrides,
  };
}

/**
 * Renders the gate with stub children, so the assertions are about which screen
 * the gate chose rather than about the camera or the album themselves.
 */
function renderGate(initialPath: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  const router = createMemoryRouter(
    [
      {
        path: "/",
        Component: Gate,
        children: [
          { index: true, element: <div>CAMERA SCREEN</div> },
          { path: "album", element: <div>ALBUM SCREEN</div> },
          { path: "album/:photoId", element: <div>VIEWER SCREEN</div> },
          { path: "slideshow", element: <div>SLIDESHOW SCREEN</div> },
        ],
      },
      { path: "/login", element: <div>LOGIN SCREEN</div> },
    ],
    { initialEntries: [initialPath] },
  );

  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );

  return router;
}

beforeEach(() => {
  window.localStorage.clear();
  window.localStorage.setItem("auth_token", "a-valid-looking-token");
  vi.restoreAllMocks();
});

describe("the gate while the event is running", () => {
  beforeEach(() => {
    vi.spyOn(eventService, "getCurrent").mockResolvedValue(eventInfo({ is_over: false }));
  });

  it("shows the camera at /", async () => {
    renderGate("/");
    expect(await screen.findByText("CAMERA SCREEN")).toBeInTheDocument();
  });

  it.each(["/album", "/album/photo-1", "/slideshow"])(
    "redirects %s back to the camera",
    async (path) => {
      const router = renderGate(path);
      expect(await screen.findByText("CAMERA SCREEN")).toBeInTheDocument();
      await waitFor(() => expect(router.state.location.pathname).toBe("/"));
      expect(screen.queryByText("ALBUM SCREEN")).not.toBeInTheDocument();
      expect(screen.queryByText("VIEWER SCREEN")).not.toBeInTheDocument();
      expect(screen.queryByText("SLIDESHOW SCREEN")).not.toBeInTheDocument();
    },
  );
});

describe("the gate once the event is over", () => {
  beforeEach(() => {
    vi.spyOn(eventService, "getCurrent").mockResolvedValue(eventInfo({ is_over: true }));
  });

  it("redirects the camera route to the album", async () => {
    const router = renderGate("/");
    expect(await screen.findByText("ALBUM SCREEN")).toBeInTheDocument();
    await waitFor(() => expect(router.state.location.pathname).toBe("/album"));
    expect(screen.queryByText("CAMERA SCREEN")).not.toBeInTheDocument();
  });

  it.each([
    ["/album", "ALBUM SCREEN"],
    ["/album/photo-1", "VIEWER SCREEN"],
    ["/slideshow", "SLIDESHOW SCREEN"],
  ])("lets %s through", async (path, expected) => {
    renderGate(path);
    expect(await screen.findByText(expected)).toBeInTheDocument();
  });
});

describe("the gate ignores the local clock", () => {
  it("stays on the camera when ends_at has passed but the server says otherwise", async () => {
    // The device clock is well past ends_at. Only is_over decides.
    vi.setSystemTime(new Date("2027-01-01T00:00:00Z"));
    vi.spyOn(eventService, "getCurrent").mockResolvedValue(
      eventInfo({ ends_at: "2026-09-14T23:59:59+02:00", is_over: false }),
    );

    renderGate("/album");
    expect(await screen.findByText("CAMERA SCREEN")).toBeInTheDocument();
    vi.useRealTimers();
  });

  it("opens the album when ends_at is in the future but the server says it is over", async () => {
    vi.spyOn(eventService, "getCurrent").mockResolvedValue(
      eventInfo({ ends_at: "2099-01-01T00:00:00+02:00", is_over: true }),
    );

    renderGate("/");
    expect(await screen.findByText("ALBUM SCREEN")).toBeInTheDocument();
  });
});

describe("the gate and the session", () => {
  it("sends a signed-out visitor to the login screen", async () => {
    window.localStorage.clear();
    renderGate("/");
    expect(await screen.findByText("LOGIN SCREEN")).toBeInTheDocument();
  });

  it("sends an expired session to the login screen", async () => {
    vi.spyOn(eventService, "getCurrent").mockRejectedValue(new ApiError(401, "invalid or expired token"));
    renderGate("/");
    expect(await screen.findByText("LOGIN SCREEN")).toBeInTheDocument();
  });

  it("keeps a 423 off the login screen: it is not an auth failure", async () => {
    vi.spyOn(eventService, "getCurrent").mockRejectedValue(
      new ApiError(423, "the event is not over yet"),
    );
    renderGate("/");
    expect(await screen.findByText(/Could not reach the server/)).toBeInTheDocument();
    expect(screen.queryByText("LOGIN SCREEN")).not.toBeInTheDocument();
  });
});
