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

## Android modules

### Account

Owns a configured server, credentials, session renewal, and logout cleanup.

### Catalog

Owns locally cached library metadata and browsing behavior.

### Downloads

Owns a durable download queue, range resumption, integrity verification, local
file lifecycle, and storage-pressure failures.

### Reader

Presents one small interface to the rest of the app while selecting a
format-specific EPUB or PDF implementation internally. It emits stable reading
locations rather than exposing engine details to synchronization code.

### Progress

Persists reading state locally first, queues synchronization, and reconciles
server state after reconnecting. Opening and reading a downloaded book never
waits on this module's network work.

## Important seams

- **HTTP contract:** the only interface the Android application learns about
  server behavior. It is versioned from its first release.
- **Book content:** represented by a server-issued edition ID plus verified
  bytes, not by a server filesystem path.
- **Reader location:** a tagged, format-specific value. EPUB locations and PDF
  pages are deliberately not flattened into one string.
- **Persistence:** domain modules receive storage dependencies; they do not
  create database connections inside domain behavior.

## Technology decision still open

Before generating framework code, choose the server and administration stack.
The preferred starting options are:

1. **Kotlin end to end:** Ktor server, PostgreSQL or SQLite, and Jetpack Compose
   Android. This maximizes language and model sharing.
2. **Go server + Kotlin Android:** a small server binary and native Android app.
   This favors simple self-hosting and explicit contracts over shared code.
3. **TypeScript server/web + Kotlin Android:** fast web administration work and
   a broad ecosystem, with a somewhat heavier server toolchain.

Regardless of choice, client/server types should be generated from the versioned
contract rather than maintained by hand in two languages.
