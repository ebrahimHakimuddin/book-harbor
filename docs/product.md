# Product definition

## Problem

People can own ebook files without having a good way to serve them privately,
read them offline on a phone, and keep their place across devices. Existing
solutions often make either hosting easy or reading pleasant, but not both.

BookHarbor combines a low-maintenance, self-hosted library with a reader-first
Android application.

## Users

### Host

The host deploys the server, owns its storage, manages accounts, and imports
books. A host may also be a reader.

### Reader

A reader browses books shared by the host, downloads them, reads offline, and
chooses whether friends may see their activity.

## Version 0.1: the complete reading loop

Version 0.1 supports EPUB and PDF. A successful release satisfies all of the
following:

- A new instance can be started from documented configuration without a cloud
  dependency.
- The first administrator can create an account safely.
- An administrator can upload an EPUB or PDF and edit its basic metadata.
- A reader can sign in and browse the books they may access.
- Android can download a complete original file and verify its integrity.
- A downloaded book opens and remains readable with no network connection.
- EPUB chapters use continuous vertical scrolling and require an explicit
  next-chapter action at each chapter boundary.
- A reader can see or hide progress and adjust theme, typeface, font size,
  spacing, margins, alignment, and brightness without a network connection.
- EPUB location and PDF page progress survive application restarts.
- Progress synchronizes after connectivity returns without silently discarding
  a newer position.
- A reader can remove a local download without deleting the server copy.
- A host can export original book files and application data.

## Explicitly deferred

- Highlights, annotations, and bookmarks
- OPDS compatibility
- Audiobooks, comics, Kindle formats, and format conversion
- Public registration and federation between servers
- iOS and desktop-native clients

Deferral means these features may influence data ownership and privacy design,
but they must not enlarge the first implementation milestone.

## Non-functional requirements

- Book content is never sent to a third party by default.
- Stored passwords use a memory-hard password hash.
- Download endpoints support resumable transfers.
- The server remains useful on modest hardware and does not require a cluster.
- Destructive actions are explicit and auditable.
- Accessibility is a release requirement for both administration and reading.
