# italy-trip

A single-page camera for one event and one shared account.

While the trip is running the app is **only a camera**: a live viewfinder, a
shutter, and no way to see any photo — not yours, not anyone's, not the one from
five minutes ago. When the event's `ends_at` passes, the camera disappears and
the whole set becomes an album with a grid, a fullscreen viewer, a slideshow and
downloadable originals.

The lock is the product, so it is enforced **on the server**. The API refuses to
emit photo bytes or photo URLs before `ends_at` and answers `423 Locked` instead.
A client with devtools open and a wrong system clock still sees nothing.

## Start

```bash
# 0. Init (once, after forking/cloning): renames placeholders, installs deps, sets up git
python3 scripts/init.py "My Project"

# 1. Database and object storage
cd backend && docker compose up -d

# 2. Backend
cp .env.example .env
go run ./cmd
# -> http://localhost:8080

# 3. Frontend
cd ../frontend
cp .env.example .env
npm install
npm run dev
# -> http://localhost:5173
```

The backend refuses to boot without `JWT_SECRET`, `EVENT_NAME`,
`EVENT_STARTS_AT`, `EVENT_ENDS_AT`, `SEED_USER_EMAIL`, `SEED_USER_PASSWORD` and
`AWS_S3_BUCKET_NAME`. That is deliberate: a silent default for any of them is
either a forgeable session or a wrong unlock date.

> **The camera needs HTTPS.** `navigator.mediaDevices` is only *defined* on
> `https://`, `localhost` or `127.0.0.1`. On `http://192.168.x.x` it is
> `undefined`, so testing the camera on a phone means using the deployed URL, not
> the LAN address.

## How the lock works

| Method | Path | While `now < ends_at` | Once `now >= ends_at` |
|---|---|---|---|
| `GET` | `/events/current` | 200, `is_over:false`, no URLs | 200, `is_over:true` |
| `POST` | `/events/current/photos` | 201 (or 200 if duplicate) | **423 Locked** |
| `GET` | `/events/current/photos` | **423 Locked** | 200, list with presigned URLs |
| `GET` | `/photos/:id/original` | **423 Locked** | 200, the bytes as an attachment |

`events.Event.IsOver(now)` in `internal/events/service.go` is the single source
of truth, and the boundary is inclusive: at exactly `ends_at`, the album opens.
The bucket has no public access — every read is a presigned URL minted by the Go
service, and only once the event is over.

There is **no registration endpoint**. With one shared account and an album that
unlocks for any authenticated user, open sign-up would let a stranger register
today and read everything later. The shared user is seeded from the environment
on boot.

## Event configuration

The event is synced from the environment on **every boot** (it upserts a
singleton row), so the unlock date is a Railway variable you change with a
redeploy rather than manual SQL. Timestamps must be RFC3339 **with an offset** —
a bare date is rejected at boot:

```
EVENT_NAME="Italy Trip"
EVENT_STARTS_AT=2026-09-05T00:00:00+02:00
EVENT_ENDS_AT=2026-09-14T23:59:59+02:00
```

## Offline captures

Captures go to IndexedDB before anything is sent, so the shutter works in a
tunnel with the tab closed. A background uploader drains the queue one at a time
with exponential backoff (2s, 4s, 8s, … capped at 5 minutes), triggered on
enqueue, on `online`, on the tab becoming visible, and on a 60s sweep.

Each capture carries a `client_id` generated once and reused on every retry.
`UNIQUE (event_id, client_id)` on the server means a request that succeeded but
whose response was lost answers 200 instead of storing a second copy.

## Deploying to Railway

The two services need to know each other's public URL. Railway's domain
variables are **bare hostnames** — `${{Frontend.RAILWAY_PUBLIC_DOMAIN}}` expands
to `app-production.up.railway.app`, with no `https://` on the front — so both
sides normalise what they are given and prepend the scheme:

```
# backend service
CORS_ORIGIN=${{Frontend.RAILWAY_PUBLIC_DOMAIN}}

# frontend service — a BUILD argument, since Vite bakes it in at build time
VITE_API_URL=${{Backend.RAILWAY_PUBLIC_DOMAIN}}
```

Either form works; a full `https://…` URL is passed through unchanged. Both
accept a comma-separated list on the backend side, so local development and the
deployed frontend can be allowed at once.

Two things to check when the app loads but every request fails:

- **The backend logs its resolved CORS allowlist at boot.** If it says
  `resolved to no usable origins`, the variable reference did not expand —
  check the service name and that the referenced service actually has a domain.
- **A CORS failure is invisible server-side.** The response is a normal 200 that
  the browser then refuses to hand to the page, so the server logs look healthy
  while the app is completely broken.

## Backend

- Entry point: `cmd/main.go` + `cmd/api.go`
- DB migrations run automatically on startup (`internal/postgres/migrations`, goose)
- Queries are sqlc-generated from `internal/postgres/sqlc/*.sql` — edit the `.sql`, then run `sqlc generate`, never edit generated files
- `internal/events` — the event domain, the `IsOver` rule, and boot-time seeding
- `internal/photos` — upload and album handlers; every route asks `IsOver` first
- `internal/storage` — the `Storage` interface, an S3 implementation, and an in-memory fake for tests
- `internal/images` — JPEG thumbnails (400px long edge, CatmullRom, quality 80)
- `internal/auth` = password hashing + JWT; `internal/middleware` = `RequireAuth` and the CORS allowlist
- `GET /health` for liveness
- Config via env vars, see `.env.example`

```bash
go test ./...                       # unit and handler tests, no network needed
go test ./internal/storage -run S3  # skipped unless AWS_* is set; hits a real bucket
```

The storage integration test asserts that an **unsigned** GET of a stored object
returns 403. That check is what proves the bucket is private; do not skip it when
pointing the app at a new bucket.

## Frontend

- React Router v8, framework mode, SPA (`ssr: false`) — no server rendering
- Tailwind v4 via `@tailwindcss/vite`, no component library
- Folder convention: `components/`, `hooks/`, `routes/`, `services/`, `types/`, `lib/`
- Data fetching = TanStack Query; API calls go through `lib/apiClient.ts`
- Auth token stored in `localStorage` (`lib/auth.ts`)
- `routes/gate.tsx` is the lock gate — it routes off the server's `is_over`, never off a local clock
- `lib/capture.ts` reads `videoWidth`/`videoHeight` at capture time, not from the constraints
- `lib/photoQueue.ts` + `lib/uploader.ts` are the durable capture queue
- `VITE_API_URL` in `.env` points at the backend

```bash
npm test        # vitest
npm run typecheck
```

## Build

- `build/Dockerfile.backend` — multi-stage Go build, alpine runtime
- `build/Dockerfile.frontend` — multi-stage Node build, served via `react-router-serve` (`npm run start`); needs `VITE_API_URL` as a build arg, since Vite bakes it in at build time
- Build both from the repo root: `docker build -f build/Dockerfile.backend .` / `docker build -f build/Dockerfile.frontend .`

## Tech stack

**Backend:** Go, Gin, pgx, goose, sqlc, golang-jwt, bcrypt, aws-sdk-go-v2, `golang.org/x/image`, Postgres

**Frontend:** React, React Router v8, TypeScript, Vite, Tailwind CSS v4, TanStack Query, react-webcam, idb

**Build:** Docker, docker compose (Postgres + MinIO for local object storage)
