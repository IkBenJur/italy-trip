# Italy Trip — Locked Event Camera

## 1. What it is

A single-page web app for one event (our Italy trip) and one shared account.

While the event is running, the app is **only a camera**. You open it, you get a live
viewfinder in the page, you take photos. You cannot see any photo you or anyone else has
taken — not even the ones from five minutes ago.

When the event's end timestamp passes, the app flips: the camera is gone, uploads are
rejected, and the whole set becomes a photo album with a grid, a fullscreen viewer, an
autoplaying slideshow, and downloadable originals.

The lock is the entire point of the product, so it is enforced **on the server**, not in the
UI. The API refuses to emit photo bytes or photo URLs before `ends_at`. A client with
devtools open and a wrong system clock still sees nothing.

## 2. Decisions (locked)

| Decision | Choice | Consequence |
|---|---|---|
| Camera | In-page `getUserMedia` via `react-webcam` | ~1080p photos, not 12MP. Requires HTTPS. |
| Storage | Railway Storage Bucket (S3-compatible, Tigris-backed) | Already provisioned as `italy-trip-bucket-prod`. Free egress. Presigned URLs, issued only after unlock. |
| Event end | Fixed `ends_at` timestamp | Set by env var, synced on boot. No manual button. |
| Visibility | Fully locked for everyone | Including the uploader. No exceptions. |
| Accounts | One shared login | No per-photo attribution in the UI. |
| Post-capture | Client-side confirm/retake | Never re-viewable once uploaded. |
| Bad signal | IndexedDB queue + background retry | Captures survive tunnels, dead zones, closed tabs. |
| Album | Grid + fullscreen + slideshow + download | No delete. |
| Deploy | Two Railway services (already provisioned) | Matches existing Dockerfiles. |
| Dev testing | Deploy to Railway to test on phone | No local tunnel or cert setup. |

## 3. Research findings

### 3.1 Camera API

Two viable approaches exist in modern browsers:

- `<input type="file" accept="image/*" capture="environment">` — opens the native camera app,
  returns a full-resolution (12MP) photo. Works everywhere. iOS transcodes HEIC to JPEG
  automatically, provided `image/heic` is **not** in `accept` (Safari 17+ will convert *to*
  HEIC if it is).
- `navigator.mediaDevices.getUserMedia()` + `<video>` + `<canvas>` — an in-page viewfinder
  with a custom shutter. Supported in Safari since iOS 11. **This is what we are building.**

Constraints that shape the implementation:

- **Secure context required.** `navigator.mediaDevices` is only *defined* on `https://`,
  `localhost`, or `127.0.0.1`. On `http://192.168.x.x` it is `undefined` and the code throws
  a `TypeError` before any permission dialog. This is not a permission you can grant — the API
  is absent. Railway serves HTTPS on `*.up.railway.app`, so production is fine; local phone
  testing over LAN IP is impossible, which is why we test on the deployed URL.
- **`ImageCapture.takePhoto()` is not implemented in Safari.** Capture must go through
  `canvas.drawImage(video, ...)`, which is capped at the *video stream* resolution
  (~1920x1080), not the camera sensor. This is the quality cost of the in-page approach.
- **iOS home-screen PWA camera bugs are real and documented**: permission not persisted across
  launches, repeated prompts, camera failing to open from the home screen, and streams dying on
  SPA route changes. Mitigation: use the app from Safari proper rather than "Add to Home
  Screen", and unmount the camera component cleanly on navigation.

### 3.2 Is image processing needed?

**Much less than with a file upload, because of the in-page choice.** A frame drawn from a
`<video>` element is a raw RGBA bitmap already in display orientation. It carries:

- no EXIF (so no orientation bug to correct, and no metadata to strip)
- no HEIC (so no decoder needed on either client or server — a real win, since Go has no good
  HEIC decoder and Chrome desktop cannot decode HEIC either)
- no multi-megabyte file (nothing was ever encoded by the camera app)

So the client pipeline is exactly one step: `canvas.toBlob("image/jpeg", 0.92)`, producing
roughly 200–400 KB at 1080p.

Server-side, one step is genuinely needed: a **thumbnail** (400px long edge) so the album grid
does not pull 60 full-size JPEGs. Done with stdlib `image/jpeg` plus
`golang.org/x/image/draw` (CatmullRom). We are *not* using `disintegration/imaging` — its last
release was 2019.

Because there is no EXIF, capture time must be recorded by the client and sent explicitly as
`taken_at`.

### 3.3 Library selection

`react-webcam@7.2.0` — 470k weekly downloads, **zero runtime dependencies**, peer range
`react >=16.2.0` so it installs cleanly against React 19. Ships its own TypeScript types. Last
published Oct 2023, which is acceptable for a thin `getUserMedia` wrapper with no dependency
surface to rot.

Rejected: `react-camera-pro` (peer-pinned to React 18, would need `--force`), `@webcam/react`
(peer React 17/18), `react-webcam-pro` (maintained, React 19 OK, but requires
`styled-components` — unwanted in a Tailwind-only project).

We use `react-webcam` for stream lifecycle, permissions, and `facingMode` handling, but do
**our own** capture off `webcamRef.current.video` rather than its `getScreenshot()`, which
returns a base64 data URL (WebP by default) and gives us no control over blob encoding.

## 4. Architecture

Two Railway services, as the existing Dockerfiles already assume.

```
  iPhone (Safari, HTTPS)
        |
        |  1. capture -> canvas -> JPEG blob -> IndexedDB queue
        |  2. background uploader drains queue
        v
  Frontend service            Backend service            Railway Bucket (private)
  React Router v7 SPA  --->   Go + Gin + sqlc      --->   photos/{id}.jpg
  react-router-serve          Postgres (Railway)          thumbs/{id}.jpg
                                     |
                                     +-- before ends_at: 423 Locked, no URLs emitted
                                     +-- after  ends_at: presigned GET URLs, 1h TTL
```

The bucket has **no public access**. Every read is a presigned URL minted by the Go service,
and the Go service only mints them once `now() >= ends_at`.

## 5. Data model

New migration `backend/internal/postgres/migrations/00002_create_event_and_photos.sql`.

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS photos (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    uploaded_by  UUID NOT NULL REFERENCES users(id),
    client_id    TEXT NOT NULL,
    storage_key  TEXT NOT NULL,
    thumb_key    TEXT NOT NULL,
    content_type TEXT NOT NULL,
    width        INTEGER NOT NULL,
    height       INTEGER NOT NULL,
    size_bytes   BIGINT NOT NULL,
    taken_at     TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, client_id)
);

CREATE INDEX IF NOT EXISTS photos_event_taken_idx ON photos (event_id, taken_at);

-- +goose Down
DROP TABLE photos;
DROP TABLE events;
```

Two things to note:

- **`TIMESTAMPTZ`, not `TIMESTAMP`.** The existing `users` table uses naive `TIMESTAMP`. For an
  event whose unlock moment must be correct while we are physically in CEST and the server runs
  UTC, a naive timestamp is a bug waiting to happen. New tables use `TIMESTAMPTZ`.
- **`UNIQUE (event_id, client_id)`** is what makes the offline retry queue safe. The client
  generates a UUID per capture and reuses it on every retry, so a request that succeeded but
  whose response was lost cannot create a duplicate photo.

## 6. API surface

All routes below `/` require the existing `RequireAuth` middleware.

| Method | Path | Locked behaviour (`now < ends_at`) | Unlocked behaviour |
|---|---|---|---|
| `GET` | `/events/current` | 200, `is_over:false`, no URLs | 200, `is_over:true` |
| `POST` | `/events/current/photos` | 201 Created (or 200 if duplicate) | **423 Locked** |
| `GET` | `/events/current/photos` | **423 Locked** | 200, list with presigned URLs |
| `GET` | `/photos/:id/original` | **423 Locked** | 302 to presigned URL |

`GET /events/current` response while locked:

```json
{ "id": "…", "name": "Italy Trip", "starts_at": "…", "ends_at": "…",
  "is_over": false, "photo_count": 42 }
```

`photo_count` is deliberately exposed while locked — it is a count, not content, and "42 photos
waiting" is a nice thing to see on the camera screen.

`GET /events/current/photos` once unlocked:

```json
{ "photos": [ { "id": "…", "taken_at": "…", "width": 1920, "height": 1080,
                "url": "https://…signed…", "thumb_url": "https://…signed…" } ] }
```

`423 Locked` is the correct status here (RFC 4918) and is distinguishable from a 401/403 auth
failure, which matters for the frontend's routing logic.

## 7. Environment variables

Infrastructure is already provisioned on Railway: two services, Postgres, and a Railway Storage
Bucket named `italy-trip-bucket-prod`. Below, `[set]` marks variables already present on the
backend service, `[MISSING]` marks ones this plan requires that are not there yet.

Backend:

```
# [set]
PORT="8080"
GIN_MODE="release"
GOOSE_DRIVER="postgres"
GOOSE_MIGRATION_DIR="./internal/postgres/migrations"
GOOSE_DBSTRING="host=${{Postgres.PGHOST}} user=${{Postgres.PGUSER}} password=${{Postgres.PGPASSWORD}} dbname=${{Postgres.PGDATABASE}}"

# [set] — the Railway Bucket service, mapped onto the names the AWS SDK reads natively.
AWS_ACCESS_KEY_ID="${{italy-trip-bucket-prod.ACCESS_KEY_ID}}"
AWS_DEFAULT_REGION="${{italy-trip-bucket-prod.REGION}}"
AWS_ENDPOINT_URL="${{italy-trip-bucket-prod.ENDPOINT}}"
AWS_S3_BUCKET_NAME="${{italy-trip-bucket-prod.BUCKET}}"
AWS_SECRET_ACCESS_KEY="${{italy-trip-bucket-prod.SECRET_ACCESS_KEY}}"

# [MISSING] — auth. JWT_SECRET is the urgent one: see the warning below.
JWT_SECRET=…
JWT_TTL_HOURS=720            # long TTL; re-logging in on a phone mid-trip is friction

# [MISSING] — the event itself. Without these the app cannot seed or unlock.
EVENT_NAME="Italy Trip"
EVENT_STARTS_AT=2026-09-05T00:00:00+02:00
EVENT_ENDS_AT=2026-09-14T23:59:59+02:00   # RFC3339 WITH offset

# [MISSING] — the shared login. Without these there is no way in.
SEED_USER_EMAIL=…
SEED_USER_PASSWORD=…

# [MISSING] — optional, both have working defaults in code.
CORS_ORIGIN=https://…up.railway.app
MAX_UPLOAD_BYTES=15728640    # 15 MB
```

> **Set `JWT_SECRET` on Railway before anything else ships.** `cmd/main.go` reads it with
> `env.GetEnv("JWT_SECRET", "dev-secret-change-me")`, so with the variable absent the production
> service silently signs tokens with a default string that is committed to this repo. Anyone who
> reads the repo can forge a valid session. Step 1 changes this to a hard boot failure rather
> than a silent fallback.

Two smaller notes on `GOOSE_DBSTRING` as currently set: it omits `port=${{Postgres.PGPORT}}`
(harmless while `PGHOST` resolves to the private `.railway.internal` host on 5432, wrong the
moment it points at the public proxy) and omits `sslmode` (pgx defaults to `prefer`, which is
fine on the private network). Worth adding both for robustness, not urgent.

Frontend: `VITE_API_URL` only.

The event is **synced from env on every boot** (upsert the singleton row). That makes the
unlock date a Railway variable you can change with a redeploy, rather than something requiring
manual SQL.

## 8. Implementation steps

Each step lists what to build and how to prove it works. Steps are ordered so the thing is
runnable and verifiable at every point.

---

### Step 1 — Dependencies and config

**Build.**
- Backend: `go get github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/s3 golang.org/x/image`
- Frontend: `npm i react-webcam idb`
- Extend `backend/internal/env/env.go` with typed getters for the new vars, including a
  `MustTime(key)` that parses RFC3339 and **panics on boot** if `EVENT_ENDS_AT` is malformed.
- Tighten CORS in `cmd/api.go`: replace `Access-Control-Allow-Origin: *` with `CORS_ORIGIN`.

**Verify.**
- `go build ./...` and `npm run typecheck` both pass.
- `go test ./internal/env` — table test: valid RFC3339 with offset parses to the expected UTC
  instant; a bare date and an empty string both fail.
- Start the server with `EVENT_ENDS_AT=garbage` and confirm it refuses to boot with a clear
  message. A silently-wrong unlock date is the worst possible failure here.

---

### Step 2 — Migration and sqlc queries

**Build.**
- Write `00002_create_event_and_photos.sql` as in §5.
- Add `backend/internal/postgres/sqlc/event_queries.sql`: `UpsertSingletonEvent`,
  `GetCurrentEvent`, `CountPhotos`.
- Add `backend/internal/postgres/sqlc/photo_queries.sql`: `CreatePhoto`,
  `FindPhotoByClientId`, `ListPhotosByEvent`, `FindPhotoById`.
- Run `sqlc generate`. Never hand-edit the generated files.

**Verify.**
- `docker compose up -d && go run ./cmd` applies the migration on boot with no error.
- `psql -c '\d photos'` shows `timestamptz` columns and the unique constraint.
- Insert two rows with the same `(event_id, client_id)` by hand and confirm the second is
  rejected — this constraint is load-bearing for the retry queue.
- `sqlc generate` produces no diff on a second run.

---

### Step 3 — Event domain, seeding, and the lock rule

**Build.**
- `backend/internal/events/service.go` with the single source of truth:
  `func (e Event) IsOver(now time.Time) bool { return !now.Before(e.EndsAt) }`
- `backend/internal/events/seed.go` — on boot, upsert the singleton event from env, and create
  the shared user from `SEED_USER_EMAIL`/`SEED_USER_PASSWORD` if absent.
- **Remove the `POST /auth/register` route.** With a shared account and an album that unlocks
  for any authenticated user, an open registration endpoint means a stranger can register now
  and read the whole album later. Login stays; registration goes.
- `backend/internal/events/handler.go` — `GET /events/current`.

**Verify.**
- `go test ./internal/events` on `IsOver`, covering: one second before `ends_at` (false),
  exactly `ends_at` (**true** — the boundary is inclusive), one second after (true), and a
  timestamp in a different offset that maps to the same UTC instant.
- Boot twice against the same database and confirm exactly one event row and one user row —
  seeding must be idempotent.
- Change `EVENT_ENDS_AT`, reboot, confirm the row updated.
- `curl -i .../auth/register` returns 404.
- `curl` `/events/current` without a token returns 401.

---

### Step 4 — Storage package

**Build.**
- `backend/internal/storage/storage.go` defining an interface, so handlers never touch the AWS
  SDK directly and tests need no network:
  ```go
  type Storage interface {
      Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error
      PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
  }
  ```
- `s3.go` implementing it with `aws-sdk-go-v2` and `s3.NewPresignClient`. Note that
  `config.LoadDefaultConfig(ctx)` reads `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
  `AWS_DEFAULT_REGION` **and** `AWS_ENDPOINT_URL` natively, so no manual endpoint wiring is
  needed — the Railway Bucket variables are picked up as-is. Read the bucket name from
  `AWS_S3_BUCKET_NAME` yourself; that one is not an SDK variable.
- If Railway's endpoint rejects virtual-hosted-style requests, set `UsePathStyle: true` on the
  S3 client. Determine this empirically in the first test below rather than guessing.
- `fake.go` — an in-memory implementation for handler tests.

**Verify.**
- `go test ./internal/storage` against the real `italy-trip-bucket-prod` bucket (it is free to
  operate and the objects are namespaced under a `test/` prefix): put an object, presign it,
  `http.Get` the presigned URL, assert the bytes round-trip. This is also where path-style
  addressing gets settled.
- Assert an **unsigned** GET of the same key returns 403 — this is the check that proves the
  bucket is actually private. Do not skip it.
- Assert a presigned URL with a 1-second TTL returns 403 after sleeping 2 seconds.

---

### Step 5 — Thumbnailing

**Build.**
- `backend/internal/images/thumbnail.go`: decode JPEG, scale so the long edge is 400px
  preserving aspect ratio, resample with `draw.CatmullRom`, re-encode at quality 80.
- Return the decoded original's width/height too, so the handler can persist real dimensions
  rather than trusting the client.

**Verify.**
- `go test ./internal/images` with generated test images: landscape 1920x1080 → 400x225;
  portrait 1080x1920 → 225x400; square 1000x1000 → 400x400; an image already smaller than 400px
  is **not** upscaled.
- Feed it a PNG and a truncated/garbage byte slice; both must return an error, not panic.

---

### Step 6 — Upload endpoint

**Build.** `POST /events/current/photos`, `multipart/form-data` with `file`, `client_id`,
`taken_at`.

Order of operations matters:
1. Load event; if `IsOver(now)` → **423**. This is the "no more pictures after the event" rule.
2. Enforce `MAX_UPLOAD_BYTES` with `http.MaxBytesReader` *before* reading the body.
3. Sniff the real content type with `http.DetectContentType` on the first 512 bytes. Reject
   anything that is not `image/jpeg`. Never trust the client's declared header.
4. Look up `client_id`; if a photo already exists, return **200** with the existing id and do no
   further work. Idempotent retry.
5. Decode, build the thumbnail, `Put` both objects (`photos/{uuid}.jpg`, `thumbs/{uuid}.jpg`).
6. Insert the row. Return **201**.

**Verify.** Handler tests with a fake `Querier` and the fake `Storage`:
- Happy path → 201, one row, two objects in fake storage.
- Same `client_id` twice → 201 then 200, still exactly one row and two objects.
- `now` past `ends_at` → 423, zero rows, zero objects.
- 20MB body → 413, and confirm nothing was written to storage.
- A PNG, and a `.jpg` that is actually text → 400.
- No auth token → 401.

Then end-to-end with curl against a local server:
```
curl -F file=@test.jpg -F client_id=$(uuidgen) -F taken_at=2026-09-06T14:00:00+02:00 \
     -H "Authorization: Bearer $TOKEN" localhost:8080/events/current/photos
```

---

### Step 7 — Album endpoints

**Build.**
- `GET /events/current/photos` — 423 if not over; otherwise list ordered by `taken_at`, each
  with presigned `url` and `thumb_url` (1h TTL).
- `GET /photos/:id/original` — 423 if not over; otherwise 302 to a presigned URL with
  `response-content-disposition: attachment`, which is what makes "download original" save to
  the camera roll rather than opening in a tab.

**Verify.**
- **The critical test:** with `ends_at` in the future, assert the response body of
  `/events/current/photos` is a 423 whose body contains **no S3 key and no signed URL**. Grep
  the raw bytes for the bucket name and for `X-Amz-Signature`. This is the test that proves the
  product works; write it first and make sure it fails before the guard exists.
- Same for `/photos/:id/original`.
- With `ends_at` in the past, both return data, and the presigned URLs actually fetch.
- Photos are ordered by `taken_at` ascending, not by upload order — with a retry queue those
  differ, and the album should read chronologically.

---

### Step 8 — Frontend: session, lock gate, camera

**Build.**
- `app/types/event.types.ts`, `app/services/event.service.ts`, `app/hooks/useEvent.ts`
  (TanStack Query, `refetchInterval: 60_000` so the app flips to album mode on its own when the
  timer passes).
- Route restructure in `app/routes.ts`: `/` (camera), `/album`, `/album/:photoId`, `/slideshow`,
  `/login`.
- A gate component: if `!is_over` → camera, and `/album*` redirects to `/`. If `is_over` →
  camera route redirects to `/album`. **Drive this off the server's `is_over`, never off a
  client-side `Date.now()` comparison** — the countdown display is cosmetic, the gate is not.
- `app/components/CameraView.tsx`:
  ```tsx
  <Webcam
    ref={webcamRef}
    audio={false}
    playsInline
    mirrored={false}
    videoConstraints={{ facingMode: { ideal: "environment" },
                        width: { ideal: 1920 }, height: { ideal: 1080 } }}
  />
  ```
- `app/lib/capture.ts` — read `video.videoWidth`/`videoHeight` **at capture time** (not from the
  constraints, which are only a request), size a canvas to match, `drawImage`, `toBlob` as
  `image/jpeg` at 0.92.
- Confirm/retake screen holding the blob in memory with an object URL. Revoke the object URL on
  unmount — leaking these on a phone will eventually kill the tab.
- Explicit UI for the two failure modes: `navigator.mediaDevices === undefined` ("needs HTTPS")
  and a `NotAllowedError` from a denied permission, with instructions to re-enable it in Safari
  settings. Without these the camera screen is just silently blank.
- Countdown to unlock on the camera screen, plus the `photo_count`.

**Verify.**
- Set up Vitest (`npm i -D vitest jsdom @testing-library/react`) and add `npm test`.
- Unit-test the capture helper against a stub video element: a 1920x1080 source produces a
  canvas of exactly those dimensions; a 0x0 source (stream not ready) throws rather than
  uploading a blank frame.
- Unit-test the gate: `is_over:false` renders camera and redirects `/album` → `/`;
  `is_over:true` does the reverse.
- Manual, on the deployed Railway URL, on the actual iPhone:
  - Viewfinder appears and uses the **rear** camera.
  - Rotate the phone mid-session; capture again; confirm the saved image is not stretched or
    rotated. (iOS has known track-dimension swapping on orientation change — this is the check
    for it.)
  - Navigate away from `/` and back; the stream restarts rather than showing a frozen frame.
  - Lock the phone, unlock, return to the tab; the stream recovers.
  - Confirm no route anywhere shows a photo.

---

### Step 9 — Offline upload queue

**Build.**
- `app/lib/photoQueue.ts` using `idb`: an object store `pending` keyed by `clientId`, holding
  `{ clientId, blob, takenAt, attempts, lastError }`.
- `app/lib/uploader.ts`: drains the queue with concurrency 1 and exponential backoff (2s, 4s,
  8s, … capped at 5 min). Triggered on enqueue, on the `online` event, on
  `visibilitychange` → visible, and on a 60s interval.
- Delete from the queue on 201 **or** 200 (duplicate). On 423 (event ended while queued), drop
  the item and surface a message — those photos can never be accepted. On 4xx other than 423,
  stop retrying and mark failed. On 5xx/network error, keep retrying.
- A pending-count indicator on the camera screen.

**Verify.**
- Vitest with `fake-indexeddb` and a mocked `fetch`:
  - enqueue → drain succeeds → store is empty.
  - fetch rejects twice then succeeds → exactly one upload persists, item cleared, backoff
    delays observed via fake timers.
  - server returns 200 (duplicate) → item cleared, not retried forever.
  - server returns 423 → item dropped, not retried.
  - server returns 400 → item marked failed, not retried.
  - Reload simulation: enqueue, tear down the module, re-init → the item is still there.
- Manual on the phone: enable Airplane Mode, take three photos (all must be accepted by the UI),
  disable Airplane Mode, confirm the pending count drains to zero and the server has exactly
  three rows. Then repeat but **close the tab** while offline before reconnecting — reopen and
  confirm the three still upload. That is the scenario this whole step exists for.

---

### Step 10 — Album, viewer, slideshow, download

**Build.**
- `/album` — responsive grid of `thumb_url`, `loading="lazy"`, aspect-ratio boxes so the layout
  does not jump as images arrive.
- `/album/:photoId` — fullscreen viewer as a real route (so the phone's back gesture closes it),
  swipe/arrow navigation, preloading the neighbouring photos.
- `/slideshow` — autoplay advancing every ~4s, play/pause, wraps at the end. Preload the next
  image before advancing so it does not flash white. Keep the screen awake with the Wake Lock
  API where available (`navigator.wakeLock`), degrading silently where it is not — Safari
  support is uneven.
- Download: a link to `/photos/:id/original` in the viewer, plus "download all" iterating the
  list. (Do not build a server-side zip; at this scale it is not worth it.)

**Verify.**
- Vitest: grid renders N tiles for N photos; the viewer route resolves the right photo by id and
  the prev/next controls are correctly disabled at the ends.
- Manual on the phone after flipping `EVENT_ENDS_AT` into the past:
  - Grid loads, thumbnails are sharp on a retina screen (this validates the 400px choice).
  - Fullscreen viewer opens, swipes both ways, back gesture returns to the grid.
  - Slideshow advances without white flashes and the screen does not dim.
  - Download saves a full-size JPEG to the camera roll.
- Set `EVENT_ENDS_AT` back into the future and confirm the album is inaccessible again — the
  lock must be reversible, since that is how we will be testing it.
- **Final end-to-end smoke test** on the existing Railway deployment, on the phone, over HTTPS:
  log in → capture → confirm → the photo uploads → `/album` is refused with 423 → flip
  `EVENT_ENDS_AT` into the past → redeploy → the album shows the photo. This is the only check
  that exercises the real S3 bucket, the real Postgres, and Safari together.

## 9. Risks and open items

- **Photo quality.** ~2MP from the video stream versus 12MP from the native camera app. Fine on
  screen, visibly soft in print. If this turns out to matter after testing on a real phone, the
  fallback is a `<input capture>` button alongside the viewfinder, which needs the EXIF and HEIC
  handling described in §3.2 that we are currently avoiding.
- **iOS orientation.** Track dimensions can swap on rotation in some iOS versions. Step 8's
  rotation check is the gate on this; if it misbehaves, lock the capture UI to portrait.
- **Losing the unlock date.** `EVENT_ENDS_AT` is synced from env on every boot, so a typo in a
  Railway variable silently changes when the album opens. The boot-time parse guard in Step 1
  catches malformed values but not a wrong-but-valid date.
- **One shared account.** Anyone with the password sees everything after unlock. Acceptable for
  two people; the removed registration endpoint is what keeps it that way.
- **Bucket is environment-scoped.** Railway gives each environment its own bucket instance with
  isolated credentials, so a staging environment would not see production photos. Good default,
  but it means the album is empty in any environment other than the one we actually shoot in.
- **Presigned URL lifetime.** A URL minted after unlock stays valid for its full hour even if
  shared. Irrelevant post-unlock, since the album is meant to be seen by then.
