<p align="center">
  <img src="brandkit/logo-mark-color.png" alt="BookHarbor logo: a lighthouse rising from an open book" width="160">
</p>

<h1 align="center">BookHarbor</h1>

<p align="center"><em>Your library. Your harbor. Every device.</em></p>

<p align="center">
  <a href="https://www.buymeacoffee.com/kidfury" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me a Coffee" height="60" width="217"></a>
</p>

BookHarbor is a self-hosted ebook library with a native Android reader. The host
keeps the books and everyone's reading data on their own server; readers get
offline downloads, a comfortable reader, and their place kept across devices.

> **Self-hosted software only. No content is provided or distributed.**
> BookHarbor is a program you run on your own server for your own library. This
> project does not host, provide, sell, or distribute any books, web novels, or
> other content, and runs no public service. Every book in an instance is added
> by the person running it, who is responsible for having the right to store and
> read it. The optional Shelfmark and novelarchive.cc integrations only connect
> to services the operator chooses to configure; they are not affiliated with or
> endorsed by this project.

## Features

**Library and hosting**

- EPUB and PDF books, plus MOBI, AZW, and AZW3, which are converted to EPUB on
  import (needs Calibre, included in the default Docker image).
- Title, authors, description, series, and cover read from each EPUB; optional
  metadata search through Hardcover.
- A web admin console at `/admin/` for importing books, editing metadata,
  managing readers, and settings.
- Book files on local disk or in an S3-compatible bucket (AWS, R2, MinIO).
- Full export of books and data, and an offline `restore` command.
- Email through Resend for invites and password resets; admin alerts through
  ntfy.

**Getting books**

- Readers request books from the app; administrators fulfill requests by
  uploading a file, linking a book already in the library, or downloading one
  through a connected [Shelfmark](#shelfmark) instance.
- Web novels from novelarchive.cc can be followed: chapters are fetched into an
  EPUB, the book appears after its first 100 chapters, and ongoing novels gain
  new chapters on a schedule. Readers can request web novels too.

**Reading on Android**

- Offline reading of downloaded books, with resumable, checksum-verified
  downloads.
- EPUB as continuous vertical chapters; PDF as a continuous page list.
- Themes, typefaces, size, spacing, margins, alignment, and brightness per
  book; read-aloud, dictionary lookup, and search inside a book.
- Bookmarks, highlights, and notes, synced across devices.
- Reading progress synced through an offline-safe protocol that never loses a
  newer position.
- Home shelves, lists, tags, series, reading streaks, and a Continue Reading
  widget.
- Friends: opt-in sharing of what you're reading, and yearly reading goals.

## Run the server

With Docker, from the repository root:

```sh
cp .env.example .env
docker compose up --build
```

Then open `http://localhost:8080/admin/` to create the first administrator.
Data lives in the `bookharbor-data` volume.

The default image includes Calibre so MOBI and AZW3 can be converted. For a much
smaller image without that, build the `slim` target:

```sh
docker build --target slim -f apps/server/Dockerfile -t bookharbor:slim .
```

Or run the server directly with Go 1.26 or newer:

```sh
cd apps/server
go run ./cmd/bookharbor
```

### Configuration

The environment variables in [`.env.example`](.env.example) cover the instance
name, port, upload size limit, and the optional Hardcover token. Everything
else (email, S3 storage, notifications, Shelfmark) is set in the admin console
under **Settings**; saved values there override the matching environment
variables listed in [`docs/api.md`](docs/api.md#integrations).

The health check is `GET /healthz`.

### Backups

**Backup** in the admin console downloads one zip with every book file, the
covers, and a snapshot of the database. To restore it into an empty data
directory, with the server stopped:

```sh
bookharbor restore [--force] backup.zip
```

### Shelfmark

[Shelfmark](https://github.com/calibrain/shelfmark) is a separate self-hosted
book search and download tool. Once its URL (and login, if it has one) is
saved in Settings, a book request can be fulfilled by searching Shelfmark and
picking a file; the server imports it when the download finishes.

## Android app

The app is in `apps/android` (Kotlin, Jetpack Compose). Build a debug build
with:

```sh
cd apps/android
./gradlew :app:assembleDebug
```

A signed release build needs `apps/android/release-key.jks` and
`apps/android/.release-password`; both are ignored by git. On first launch,
enter your server's address and sign in with an account the administrator
created (there is no public sign-up).

## Repository layout

```text
apps/
  server/    HTTP server (Go), with the admin console embedded
  admin/     Admin console (React, TypeScript, Tailwind)
  android/   Android app (Kotlin, Jetpack Compose)
docs/        Architecture, API, sync, and reader documentation
brandkit/    Logo, palette, and typography references
DESIGN.md    Design system
```

See [`docs/`](docs/README.md) for how it fits together.

## Principles

- **Self-hosted.** One server process and one data directory on ordinary home
  hardware; no cloud service required.
- **Offline first.** Losing the network never interrupts an open book.
- **Your data.** Original files and reading data can always be exported; book
  content is never sent to a third party by default.
- **Private by default.** Reading activity is visible only to friends you
  approve, and only if you choose to share it.
