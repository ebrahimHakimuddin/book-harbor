# BookHarbor

BookHarbor is a self-hosted ebook library and reading platform. It is designed
around ownership: the host controls the books and user data, while readers get
a polished Android experience with offline downloads and synchronized reading
progress.

## First release

The first usable release will let a host:

1. deploy a BookHarbor server;
2. create the first administrator account;
3. upload EPUB and PDF books; and
4. invite readers to the library.

It will let a reader:

1. sign in from Android;
2. browse the library;
3. download and read a book offline; and
4. synchronize reading progress across devices.

Friend activity, reading goals, annotations, and additional formats are planned
after this core loop is dependable.

## Repository status

BookHarbor is in early development. The server provides persistent instance
discovery, one-time administrator bootstrap, and rotating authenticated
sessions. Book imports are the next server milestone. The product scope, system
shape, and first client/server contract live in [`docs/`](docs/README.md).

## Run the server

With Go 1.26 or newer:

```sh
cd apps/server
go run ./cmd/bookharbor
```

Or run the self-hosted container from the repository root:

```sh
cp .env.example .env
docker compose up --build
```

The health endpoint is `http://localhost:8080/healthz`; instance discovery is
available at `http://localhost:8080/api/v1/instance`.

## Product principles

- **Self-hosted by default.** A normal home server should be enough.
- **Offline-first reading.** Network loss must never interrupt an open book.
- **Portable data.** Original files and reading data must be exportable.
- **Format-aware readers.** EPUB locations and PDF pages are not forced into a
  lossy shared representation.
- **Private social features.** Reading activity is opt-in and visible only to
  explicitly approved friends.
- **Small operational footprint.** The default deployment should require as few
  moving parts as practical.

## Proposed workspace shape

```text
apps/
  android/       Native Android client
  server/        Self-hosted HTTP server and web administration
packages/
  contract/      Versioned client/server contract and generated types
docs/             Product, architecture, and decision records
```

The server uses Go; the Android client will use Kotlin and Jetpack Compose. The
rationale and remaining technology decisions are documented in
[`docs/architecture.md`](docs/architecture.md).
