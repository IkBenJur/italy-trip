import { type RouteConfig, index, layout, route } from "@react-router/dev/routes";

export default [
  route("login", "routes/login.tsx"),

  // Everything inside the gate is subject to the server's is_over: the camera
  // while the trip runs, the album afterwards, and never both.
  layout("routes/gate.tsx", [
    index("routes/camera.tsx"),
    route("album", "routes/album.tsx"),
    route("album/:photoId", "routes/photo.tsx"),
    route("slideshow", "routes/slideshow.tsx"),
  ]),
] satisfies RouteConfig;
