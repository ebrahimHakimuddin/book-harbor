# Initial architecture

BookHarbor begins as a modular monolith plus a native Android application. A
single deployable server keeps self-hosting approachable; internal modules keep
domain behavior from becoming coupled to HTTP, a database, or file storage.

## System shape

```text
Android client ---- versioned HTTP interface ---- BookHarbor server
                                                   |          |
                                               metadata    book files
```

The default deployment should use one server process, one metadata store, and
one book-file directory. Remote object storage can be added later through a real
storage seam once there are two required adapters.

## Server modules

### Identity

Owns accounts, sessions, invitations, and authorization decisions. Other
modules receive an authenticated actor; they do not parse tokens or inspect
passwords.

### Library

Owns books, editions, contributors, cover metadata, and reader access. Importing
a file is a deep operation behind a small interface: validate format, calculate
integrity data, extract available metadata, persist the original, and create the
edition atomically.

### Reading

Owns per-reader state. It stores format-specific locations alongside a normalized
percentage used only for display and comparison. The original locator remains
authoritative when reopening a book.

### Delivery

Owns safe, resumable access to original files and covers. It enforces Library
authorization before exposing bytes and supplies content length, range, and
integrity metadata needed by offline clients.

### Administration

Coordinates instance bootstrap, account management, imports, and export. It is
not a second copy of Identity or Library behavior.

The built-in web administration surface is served by the Go server at
`/admin/`. It speaks only to the versioned HTTP interface, so presentation code
does not depend on SQLite tables or file paths. External catalog lookups sit
behind a provider interface and return reviewable candidates; providers cannot
write local library state directly.

## Android modules

### Account

Owns a configured server, credentials, session renewal, and logout cleanup.

### Catalog

Owns locally cached library metadata and browsing behavior.

### Downloads

Owns resumable, byte-range downloads, integrity verification, and local file
lifecycle. A retry after a dropped connection or a killed process continues
from the bytes already on disk rather than starting over. There is no
background download queue yet: a download runs only while its screen is open,
and a storage-pressure failure surfaces as a retryable error rather than a
dedicated recovery path.

### Reader

Presents one small interface to the rest of the app while selecting a
format-specific EPUB or PDF implementation internally. It emits stable reading
locations rather than exposing engine details to synchronization code. EPUB
renders one isolated spine item as a continuous vertical surface and requires
an explicit transition at chapter boundaries. PDF incrementally renders a
vertical window of pages. Reader preferences and the current render location
are local dependencies, so reflow and customization do not depend on the
network. See [`reader.md`](reader.md).

### Progress

Persists reading state locally first, queues synchronization, and reconciles
server state after reconnecting. Opening and reading a downloaded book never
waits on this module's network work. Its durable outbox and server cursor are
updated transactionally as defined in [`offline-sync.md`](offline-sync.md).

## Important seams

- **HTTP contract:** the only interface the Android application learns about
  server behavior. It is versioned from its first release.
- **Book content:** represented by a server-issued edition ID plus verified
  bytes, not by a server filesystem path.
- **Reader location:** a tagged, format-specific value. EPUB locations and PDF
  pages are deliberately not flattened into one string.
- **Persistence:** domain modules receive storage dependencies; they do not
  create database connections inside domain behavior.

## Technology direction

The server is written in Go and the Android client will use Kotlin with Jetpack
Compose. The initial server uses the Go standard library where practical and is
packaged as a small container. This favors a low operational footprint and an
explicit client/server contract over sharing implementation code across tiers.

The default metadata store is SQLite in WAL mode. This preserves the one-process,
one-directory deployment model and provides transactional state without another
required container. See [decision 0003](decisions/0003-sqlite-metadata-store.md).

The administration frontend (`apps/admin`) is a responsive React app using
TypeScript, Tailwind, shadcn/ui, and TanStack Query. It is built to static files
that the server binary embeds, so a deployment is still one Go binary with no
Node runtime. Client/server types should eventually be
generated from the versioned contract rather than maintained by hand in two
languages. See [decision 0002](decisions/0002-go-server-kotlin-android.md).
