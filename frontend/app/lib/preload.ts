/**
 * Warms the browser cache for an image before it is shown, so the viewer and
 * the slideshow never flash white between photos.
 */
export function preloadImage(url: string | undefined): void {
  if (!url || typeof Image === "undefined") return;
  const image = new Image();
  image.decoding = "async";
  image.src = url;
}

/** Resolves once the image is decodable, or immediately if it fails. */
export function whenLoaded(url: string | undefined): Promise<void> {
  if (!url || typeof Image === "undefined") return Promise.resolve();

  return new Promise((resolve) => {
    const image = new Image();
    image.decoding = "async";
    image.onload = () => resolve();
    image.onerror = () => resolve();
    image.src = url;
  });
}
