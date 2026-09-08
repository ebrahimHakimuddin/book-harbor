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

BookHarbor is at the product-foundation stage. The product scope, system shape,
and first client/server contract live in [`docs/`](docs/README.md).

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

The implementation stack is intentionally not committed yet. The immediate
next decision is documented in [`docs/architecture.md`](docs/architecture.md).
