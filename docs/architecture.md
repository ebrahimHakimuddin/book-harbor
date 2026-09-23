# Architecture

BookHarbor is a modular monolith plus a native Android app. One server process
with one data directory keeps self-hosting simple; internal modules keep domain
behavior independent of HTTP, the database, and file storage.

## System shape

```text
Android app ----\                          /-- SQLite metadata (data directory)
                 versioned HTTP API ---- BookHarbor server
Admin console --/   (/api/v1)              \-- book files (disk or S3)
                                            \-- integrations: Resend, ntfy,
                                                Hardcover, Shelfmark, novelarchive.cc
```

The admin console is a React app built to static files that the Go binary
embeds and serves at `/admin/`. It uses the same HTTP API as the app, so neither
client depends on database tables or file paths.

## Server modules

All live under `apps/server/internal/`.

- **identity**: accounts, sessions, password resets, and authorization. Other
  modules receive an authenticated user; they never parse tokens or passwords.
- **library**: books, editions, metadata, and covers. Importing a file is one
  deep operation: validate the format (converting MOBI/AZW3 to EPUB first),
  checksum it, extract EPUB metadata, store the original, and create the
  edition atomically. Files live on disk or in S3 (`objectstore`); an edition's
  file can be replaced in place, which web novel updates use.
- **reading**: per-reader progress. Format-specific locators are authoritative;
  percentages are display summaries. See [`offline-sync.md`](offline-sync.md).
- **annotations**: bookmarks, highlights, and notes, synced with
  last-edit-wins and tombstones.
- **social**: friendships, friend requests, activity visibility, and reading
  goals.
- **lists**: each reader's own named book lists.
- **requests**: readers' book requests and the administrators' queue.
- **metadata**: the Hardcover search provider behind a provider interface. It
  returns candidates for review; it never writes library state.
- **shelfmark**: a client for a Shelfmark instance: search, queue, follow, and
  fetch a download.
- **webnovel**: followed web novels. A worker fetches chapters from
  novelarchive.cc's public API at a fixed pace, caches them, builds an EPUB,
  and keeps ongoing novels current on the configured interval. If the site
  stops answering its API, syncing stops with that error and retries later.
- **settings**: integration settings edited in the admin console, falling back
  to environment variables.
- **notify** and **mail**: ntfy pushes for administrators, and email through
  Resend.
- **audit**: a log of account, book, and settings changes.
- **export**: the full backup zip and the offline `restore` command.
- **httpapi**: routes, request validation, and error codes. Handlers translate
  between HTTP and the modules above.

## Android modules

Under `apps/android/app/src/main/java/dev/bookharbor/app/`.

- **library**: the server connection, session renewal, catalog (cached for
  offline use), downloads, and the Home, Browse, Lists, Requests, Friends, and
  Settings screens. Downloads resume from the bytes already on disk and are
  verified against the edition's SHA-256. A downloaded file the server has
  since replaced is fetched again in the background, keeping the old copy
  until the new one is verified.
- **reader**: EPUB and PDF readers behind one entry point, plus reader
  settings, annotations, read-aloud, and reading stats. See
  [`reader.md`](reader.md).
- **sync**: the durable progress outbox and the progress and annotation sync
  engines, run by WorkManager.
- **notify**: a periodic check that notifies about new books, fulfilled
  requests, and friend requests.
- **widget**: the Continue Reading home-screen widget.

## Seams

- **HTTP API**: the only thing each client knows about the server. It is
  versioned by route prefix; see [`api.md`](api.md).
- **Book content**: identified by a server-issued edition ID plus verified
  bytes, never by a server file path.
- **Reader location**: a tagged, format-specific value. EPUB positions and PDF
  pages are not flattened into one string.
- **Persistence**: modules receive their database and storage dependencies;
  they do not open connections themselves.

## Technology

- Server: Go, mostly the standard library, with SQLite in WAL mode as the
  metadata store, so a deployment is one process and one data directory.
- Admin console: React, TypeScript, Tailwind, shadcn/ui, and TanStack Query.
- Android: Kotlin and Jetpack Compose.
- Container: the default image is Debian with Calibre for MOBI/AZW3
  conversion; the `slim` target is the static binary alone.
