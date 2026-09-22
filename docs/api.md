# HTTP interface sketch

All routes live below `/api/v1`. JSON error responses contain a stable `code`, a
human-readable `message`, and an optional `details` object. Identifiers are
opaque strings.

This is a behavioral sketch, not yet an OpenAPI definition.

## Instance and sessions

```text
GET    /instance
POST   /bootstrap
POST   /sessions
POST   /sessions/refresh
DELETE /sessions/current
GET    /me
GET    /admin/users                    administrator only
POST   /admin/users                    administrator only
PATCH  /admin/users/{userId}           administrator only
DELETE /admin/users/{userId}           administrator only
GET    /admin/audit?limit=             administrator only
GET    /admin/export                   administrator only
GET    /admin/metadata/search?q=       administrator only
```

`/bootstrap` is available only while no administrator exists. Session creation
returns short-lived access credentials plus a renewable session appropriate for
native clients.

The bootstrap request creates the first administrator:

```json
{
  "displayName": "Harbor Master",
  "email": "reader@example.com",
  "password": "a long private password"
}
```

It returns `201 Created` once. Subsequent requests return the stable
`already_bootstrapped` conflict code.

Session creation accepts an email and password. It returns a 15-minute bearer
access token and a rotating refresh token with a 30-day lifetime. Only SHA-256
token hashes are stored by the server.

```json
{
  "email": "reader@example.com",
  "password": "a long private password"
}
```

Refresh requests exchange the current refresh token for a new access and
refresh token pair. The exchanged tokens stop working immediately:

```json
{
  "refreshToken": "bha_rt_..."
}
```

Protected routes use `Authorization: Bearer <accessToken>`. Deleting the current
session revokes both its access and refresh tokens.

Administrators create reader accounts explicitly; there is no public
registration. `POST /admin/users` accepts `displayName`, `email`, and a password
of at least 12 bytes. `GET /admin/users` returns administrators and readers but
never credential data, and includes each user's `disabled` state.

`PATCH /admin/users/{userId}` accepts any non-empty subset of `role`
(`admin` or `reader`), `disabled`, and `password`. Disabling an account or
resetting its password revokes its sessions; a disabled account cannot sign in.
Administrators cannot change their own role, disable, or delete themselves, and
the last active administrator cannot be demoted, disabled, or deleted (`409`).
`DELETE` removes the account and its reading progress but not book files.

Account and book changes (`user.create`, `user.update`, `user.delete`,
`book.import`, `book.update`, `book.delete`, `export.create`) are recorded in an
audit log returned newest first by `GET /admin/audit`. Entries never contain
passwords.

`GET /admin/export` streams a zip containing every original book file under
`books/`, `manifest.json`, and `bookharbor.db`, a consistent snapshot of the
metadata database with sessions removed. Password hashes remain in the snapshot.

## Library

```text
GET    /books?limit=&cursor=
GET    /books/{bookId}
POST   /books                         administrator only
PATCH  /books/{bookId}                administrator only
DELETE /books/{bookId}                administrator only
POST   /books/{bookId}/editions       administrator only
GET    /books/{bookId}/cover
PUT    /books/{bookId}/cover          administrator only
DELETE /books/{bookId}/cover          administrator only
GET    /editions/{editionId}/content
```

`GET /books` returns up to `limit` books (default 50, maximum 100), newest
first, plus `nextCursor` when more remain. Pass it back as `cursor` to fetch the
next page; a malformed cursor returns `400 invalid_cursor`.

`POST /books/{bookId}/editions` takes the same single-`file` multipart body as
`POST /books` and adds an EPUB or PDF to an existing book. A book holds one
edition per format; a second edition in the same format returns `409
edition_exists`.

`PUT /books/{bookId}/cover` takes a raw PNG, JPEG, or WebP body of at most
2 MiB and sets the book's `coverUrl` to the server-relative
`/api/v1/books/{bookId}/cover`. Clients resolve it against the server address
and send their bearer token, so uploaded covers are never public. `DELETE`
removes it. Covers from other sites keep their absolute `https` URL.

`POST /books` accepts `multipart/form-data` with exactly one `file` part and an
optional `title` field. The server validates the file contents rather than its
extension, extracts an EPUB title when available, and preserves the original
bytes. The default upload limit is 512 MiB and can be changed with
`BOOKHARBOR_MAX_UPLOAD_BYTES`.

An edition response includes format, byte length, media type, SHA-256 checksum,
and an authenticated content URL. Content supports `GET`, `HEAD`, and standard
byte ranges so Android can verify and resume a partial download.

`PATCH /books/{bookId}` accepts any non-empty subset of `title`, `subtitle`,
`description`, `authors`, `coverUrl`, and a reviewed external metadata `source`:

```json
{
  "title": "The Left Hand of Darkness",
  "subtitle": "A Novel",
  "authors": ["Ursula K. Le Guin"],
  "description": "...",
  "coverUrl": "https://...",
  "source": { "provider": "hardcover", "id": "12345" }
}
```

Metadata search is a suggestion workflow rather than a direct write. When
`BOOKHARBOR_HARDCOVER_TOKEN` is configured, the admin search endpoint queries
Hardcover on the server and returns provider-neutral candidates. The
administrator reviews a candidate and applies its fields through the ordinary
book patch route. Provider credentials are never exposed to the browser.

## Reading progress

```text
POST   /progress/sync
```

One authenticated request pushes locally queued events and pulls canonical
progress changed after the client's durable cursor:

```json
{
  "cursor": 41,
  "changes": [
    {
      "eventId": "01K5Q4Y87CT6C8S6AVJAWN5F4T",
      "editionId": "edition_opaque_id",
      "bookId": "book_opaque_id",
      "deviceId": "device_opaque_id",
      "occurredAt": "2026-09-21T12:00:00Z",
      "locator": {
        "kind": "epub-cfi",
        "value": "epubcfi(...)"
      },
      "percentage": 0.42
    }
  ]
}
```

The response acknowledges every pushed event and returns the current value for
books touched after the supplied cursor:

```json
{
  "cursor": 44,
  "hasMore": false,
  "acknowledgements": [
    {
      "eventId": "01K5Q4Y87CT6C8S6AVJAWN5F4T",
      "revision": 44,
      "disposition": "applied",
      "duplicate": false
    }
  ],
  "progress": [
    {
      "revision": 44,
      "eventId": "01K5Q4Y87CT6C8S6AVJAWN5F4T",
      "deviceId": "device_opaque_id",
      "bookId": "book_opaque_id",
      "editionId": "edition_opaque_id",
      "occurredAt": "2026-09-21T12:00:00Z",
      "locator": {
        "kind": "epub-cfi",
        "value": "epubcfi(...)"
      },
      "percentage": 0.42
    }
  ]
}
```

PDF uses a tagged locator such as `{ "kind": "pdf-page", "page": 84 }`.
`changes` is limited to 100 events per request. Pull pages contain at most 200
events; clients continue immediately with the returned cursor while `hasMore`
is true.

Client event IDs are idempotency keys scoped to the authenticated reader. A
retry with the same event and payload returns its original revision with
`duplicate: true`. Reusing an ID with different data returns
`event_id_conflict`. A delayed event is retained and acknowledged as
`superseded`, but cannot move canonical progress behind newer reading activity.
See [`offline-sync.md`](offline-sync.md) for the client protocol.

## Annotations

```text
POST   /annotations/sync
```

Bookmarks, highlights, and notes sync with one request. It pushes the client's
changed annotations and pulls every annotation changed after the cursor.
Clients choose the annotation IDs, and IDs are scoped to the reader.

```json
{
  "cursor": 12,
  "changes": [
    {
      "id": "ann_4f1c…",
      "bookId": "book_opaque_id",
      "kind": "highlight",
      "locator": { "kind": "epub-cfi", "value": "epubcfi(/6/4!/4/2:15)", "page": 0 },
      "endOffset": 42,
      "label": "Chapter 2",
      "excerpt": "the highlighted words",
      "note": "",
      "createdAt": "2026-09-21T12:00:00Z",
      "updatedAt": "2026-09-21T12:05:00Z",
      "deleted": false
    }
  ]
}
```

The response has the shape `{ cursor, hasMore, accepted, rejected,
annotations }`. `annotations` uses the same shape as `changes`.

- **Last edit wins.** The version with the newest `updatedAt` is kept. A change
  that loses to a newer copy on the server is still listed in `accepted`, and
  the winning copy is included in `annotations`.
- **Deletes are tombstones.** A delete is sent with `"deleted": true`, and the
  server keeps the tombstone so other devices learn about the delete.
- **Rejected changes** are invalid or refer to a deleted book. The client stops
  sending them.
- **Clock skew.** An `updatedAt` more than five minutes in the future is set to
  the server's time.
- **Highlight ranges.** For a highlight, the locator's CFI offset is where the
  passage starts inside its block, and `endOffset` is where it ends. An
  `endOffset` of 0 means the whole block.
- **Limits.** `changes` holds at most 100 annotations per request. A pull
  returns at most 500 per page; continue with the returned cursor while
  `hasMore` is true.

## Contract rules

- Timestamps are UTC RFC 3339 values. Canonical progress uses the latest reading
  time, with the server revision breaking ties. Events more than five minutes
  ahead of server time are rejected so a badly skewed clock cannot indefinitely
  dominate other devices.
- Percentages range from `0` through `1`; they are summaries, not reopen
  locations.
- Unknown response fields are ignored by clients.
- Breaking changes require a new major route prefix.
- Original-file checksums use an explicitly named algorithm, initially SHA-256.
