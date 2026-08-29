import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, Outlet, RouterProvider } from "react-router";
import Album from "~/routes/album";
import PhotoViewer from "~/routes/photo";
import Slideshow from "~/routes/slideshow";
import { photoService } from "~/services/photo.service";
import type { EventInfo, Photo } from "~/types/event.types";

const event: EventInfo = {
  id: "event-1",
  name: "Italy Trip",
  starts_at: "2026-09-05T00:00:00+02:00",
  ends_at: "2026-09-14T23:59:59+02:00",
  is_over: true,
  photo_count: 3,
};

function photo(id: string, takenAt: string, overrides: Partial<Photo> = {}): Photo {
  return {
    id,
    taken_at: takenAt,
    width: 1920,
    height: 1080,
    url: `https://signed.example.com/photos/${id}.jpg?X-Amz-Signature=abc`,
    thumb_url: `https://signed.example.com/thumbs/${id}.jpg?X-Amz-Signature=abc`,
    ...overrides,
  };
}

const photos = [
  photo("p1", "2026-09-06T10:00:00+02:00"),
  photo("p2", "2026-09-08T18:30:00+02:00", { width: 1080, height: 1920 }),
  photo("p3", "2026-09-12T09:00:00+02:00"),
];

/** Renders an album route inside the gate's outlet context. */
function renderRoute(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: <GateStub />,
        children: [
          { path: "album", Component: Album },
          { path: "album/:photoId", Component: PhotoViewer },
          { path: "slideshow", Component: Slideshow },
        ],
      },
    ],
    { initialEntries: [path] },
  );

  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );

  return router;
}

/** Stands in for the gate, supplying the event through the outlet context. */
function GateStub() {
  return <Outlet context={event} />;
}

beforeEach(() => {
  window.localStorage.setItem("auth_token", "token");
  vi.restoreAllMocks();
  vi.spyOn(photoService, "list").mockResolvedValue({ photos });
});

describe("the album grid", () => {
  it("renders one tile per photo", async () => {
    renderRoute("/album");

    const images = await screen.findAllByRole("img");
    expect(images).toHaveLength(photos.length);
  });

  it("uses the thumbnail, lazily, and never the full-size original", async () => {
    renderRoute("/album");

    const images = await screen.findAllByRole("img");
    for (const [index, image] of images.entries()) {
      expect(image).toHaveAttribute("src", photos[index]!.thumb_url);
      expect(image).toHaveAttribute("loading", "lazy");
    }
  });

  it("reserves each tile's aspect ratio so the grid does not jump", async () => {
    renderRoute("/album");
    await screen.findAllByRole("img");

    const links = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("href")?.startsWith("/album/"));

    expect(links[0]).toHaveStyle({ aspectRatio: "1920 / 1080" });
    expect(links[1]).toHaveStyle({ aspectRatio: "1080 / 1920" }); // the portrait one
  });

  it("links each tile to its own viewer route", async () => {
    renderRoute("/album");
    await screen.findAllByRole("img");

    for (const photoItem of photos) {
      expect(
        screen.getAllByRole("link").some((link) => link.getAttribute("href") === `/album/${photoItem.id}`),
      ).toBe(true);
    }
  });

  it("renders an empty album without breaking", async () => {
    vi.spyOn(photoService, "list").mockResolvedValue({ photos: [] });
    renderRoute("/album");

    expect(await screen.findByText(/No photos were taken/)).toBeInTheDocument();
    expect(screen.queryAllByRole("img")).toHaveLength(0);
  });
});

describe("the photo viewer", () => {
  it("resolves the right photo from the route id", async () => {
    renderRoute("/album/p2");

    const image = await screen.findByRole("img");
    expect(image).toHaveAttribute("src", photos[1]!.url);
    expect(await screen.findByText("2 / 3")).toBeInTheDocument();
  });

  it("disables previous on the first photo", async () => {
    renderRoute("/album/p1");

    await screen.findByRole("img");
    expect(screen.getByRole("button", { name: "Previous photo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next photo" })).toBeEnabled();
  });

  it("disables next on the last photo", async () => {
    renderRoute("/album/p3");

    await screen.findByRole("img");
    expect(screen.getByRole("button", { name: "Next photo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous photo" })).toBeEnabled();
  });

  it("enables both controls in the middle", async () => {
    renderRoute("/album/p2");

    await screen.findByRole("img");
    expect(screen.getByRole("button", { name: "Previous photo" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Next photo" })).toBeEnabled();
  });

  it("navigates to the next photo", async () => {
    const user = userEvent.setup();
    const router = renderRoute("/album/p1");
    await screen.findByRole("img");

    await user.click(screen.getByRole("button", { name: "Next photo" }));

    await waitFor(() => expect(router.state.location.pathname).toBe("/album/p2"));
    expect(await screen.findByText("2 / 3")).toBeInTheDocument();
  });

  it("navigates back with the arrow keys", async () => {
    const user = userEvent.setup();
    const router = renderRoute("/album/p2");
    await screen.findByRole("img");

    await user.keyboard("{ArrowLeft}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/album/p1"));

    await user.keyboard("{ArrowRight}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/album/p2"));
  });

  it("says so when the id is not in the album", async () => {
    renderRoute("/album/does-not-exist");
    expect(await screen.findByText(/not in the album/)).toBeInTheDocument();
  });
});

describe("the slideshow", () => {
  it("starts on the first photo and reports its position", async () => {
    renderRoute("/slideshow");

    expect(await screen.findByRole("img")).toHaveAttribute("src", photos[0]!.url);
    expect(screen.getByText("1 / 3")).toBeInTheDocument();
  });

  it("advances and wraps at the end", async () => {
    const user = userEvent.setup();
    renderRoute("/slideshow");
    await screen.findByRole("img");

    await user.click(screen.getByRole("button", { name: "Next photo" }));
    expect(await screen.findByText("2 / 3")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Next photo" }));
    expect(await screen.findByText("3 / 3")).toBeInTheDocument();

    // Wraps rather than stopping, so it runs unattended.
    await user.click(screen.getByRole("button", { name: "Next photo" }));
    expect(await screen.findByText("1 / 3")).toBeInTheDocument();
  });

  it("wraps backwards too", async () => {
    const user = userEvent.setup();
    renderRoute("/slideshow");
    await screen.findByRole("img");

    await user.click(screen.getByRole("button", { name: "Previous photo" }));
    expect(await screen.findByText("3 / 3")).toBeInTheDocument();
  });

  it("toggles between play and pause", async () => {
    const user = userEvent.setup();
    renderRoute("/slideshow");
    await screen.findByRole("img");

    const button = screen.getByRole("button", { name: "Pause" });
    await user.click(button);
    expect(await screen.findByRole("button", { name: "Play" })).toBeInTheDocument();
  });
});
