# HTTP API

All routes live below `/api/v1`. JSON error responses contain a stable `code`, a
human-readable `message`, and an optional `details` object. Identifiers are
opaque strings. Routes marked "administrator only" return `403` to readers.

## Instance and sessions

```text
GET    /instance
POST   /bootstrap
POST   /sessions
POST   /sessions/refresh
DELETE /sessions/current
POST   /password-resets
POST   /password-resets/confirm
GET    /me
GET    /admin/users                    administrator only
POST   /admin/users                    administrator only
PATCH  /admin/users/{userId}           administrator only
DELETE /admin/users/{userId}           administrator only
GET    /admin/audit?limit=             administrator only
GET    /admin/export                   administrator only
GET    /admin/metadata/search?q=       administrator only
GET    /admin/settings                 administrator only
PATCH  /admin/settings                 administrator only
POST   /admin/settings/test            administrator only
GET    /admin/storage                  administrator only
POST   /admin/storage/move-to-s3       administrator only
GET    /admin/shelfmark/search?title=&author=   administrator only
GET    /admin/shelfmark/downloads      administrator only
POST   /admin/shelfmark/downloads      administrator only
GET    /admin/webnovels                administrator only
POST   /admin/webnovels                administrator only
GET    /admin/webnovels/search?q=      administrator only
POST   /admin/webnovels/{sourceId}/sync   administrator only
DELETE /admin/webnovels/{sourceId}     administrator only
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

`POST /password-resets` takes `{"email"}` and emails a single-use,
10-character code when the address belongs to an active account. It answers
`202` either way, sends at most one code a minute per account, and returns
`422 password_reset_unavailable` when the server has no mail configured
(`GET /instance` reports this as `passwordResetEnabled`). `POST
/password-resets/confirm` takes `{"email", "code", "newPassword"}`; a code
expires after 30 minutes and is burned after five wrong guesses, and any
failure is the same `422 invalid_reset_code`. Success returns `204`, revokes
every session, and is audited as `user.password_reset`.

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
`books/` (fetched from S3 where stored there), `manifest.json`, and
`bookharbor.db`, a consistent snapshot of the metadata database with sessions and
secret settings removed and every file marked as on disk, where a restore puts it.
Password hashes remain in the snapshot.

## Integrations

Email (Resend), book file storage (S3-compatible), administrator notifications
(ntfy), Shelfmark, and the web novel update schedule are configured from the
admin console. `GET /admin/settings`
returns every key as `{ "value", "set", "secret", "source" }`; a secret's value is
never returned, only whether it is set. `PATCH /admin/settings` takes a partial
`{ "key": "value" }` map, validates all of it before saving any, and treats `""` as
"remove the saved value". A key with no saved value falls back to its environment
variable (`source: "environment"`):

| Key | Environment variable |
| --- | --- |
| `resend.apiKey` (secret) | `BOOKHARBOR_RESEND_API_KEY` |
| `resend.fromEmail` | `BOOKHARBOR_RESEND_FROM_EMAIL` |
| `resend.fromName` | `BOOKHARBOR_RESEND_FROM_NAME` |
| `s3.endpoint` | `BOOKHARBOR_S3_ENDPOINT` |
| `s3.region` | `BOOKHARBOR_S3_REGION` (default `us-east-1`) |
| `s3.bucket` | `BOOKHARBOR_S3_BUCKET` |
| `s3.accessKeyId` | `BOOKHARBOR_S3_ACCESS_KEY_ID` |
| `s3.secretKey` (secret) | `BOOKHARBOR_S3_SECRET_ACCESS_KEY` |
| `s3.prefix` | `BOOKHARBOR_S3_PREFIX` |
| `s3.pathStyle` | `BOOKHARBOR_S3_PATH_STYLE` (`true`/`false`) |
| `s3.storeUploads` | `BOOKHARBOR_S3_STORE_UPLOADS` (`true`/`false`) |
| `ntfy.url` | `BOOKHARBOR_NTFY_URL` (default `https://ntfy.sh`) |
| `ntfy.topic` | `BOOKHARBOR_NTFY_TOPIC` |
| `ntfy.token` (secret) | `BOOKHARBOR_NTFY_TOKEN` |
| `shelfmark.url` | `BOOKHARBOR_SHELFMARK_URL` |
| `shelfmark.username` | `BOOKHARBOR_SHELFMARK_USERNAME` |
| `shelfmark.password` (secret) | `BOOKHARBOR_SHELFMARK_PASSWORD` |
| `webnovels.syncHours` | `BOOKHARBOR_WEBNOVELS_SYNC_HOURS` (default `24`; `0` turns updates off) |

`POST /admin/settings/test` with `{ "integration": "email" | "s3" | "ntfy" |
"shelfmark" }` exercises the saved settings: a test email to the signed-in
administrator, a write/read/delete of a small object, a test push, or a
Shelfmark sign-in. Failures return `502` with the
provider's message.

Each edition's file is on local disk or in S3. With `s3.storeUploads` on, new
uploads go to the bucket; existing files stay where they are until
`POST /admin/storage/move-to-s3` starts a background move (`202`). Each file is
uploaded with its signed SHA-256, so the bucket rejects a corrupted copy, and is
deleted locally only after it is stored. `GET /admin/storage` reports
`{ "disk", "s3", "moving", "moved", "error" }`; a stopped move resumes where it
left off when started again. Downloads of S3 files stream through the server with
range support, so clients are unaffected. Covers always stay on disk.

ntfy receives `book_request.create` and `user.password_reset` events, the
outcome of a move to S3, finished or failed Shelfmark downloads, and web novels
added or gaining chapters.

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
`POST /books` and adds another format to an existing book. A book holds one
edition per format; a second edition in the same format returns `409
edition_exists`.

`PUT /books/{bookId}/cover` takes a raw PNG, JPEG, or WebP body of at most
2 MiB and sets the book's `coverUrl` to the server-relative
`/api/v1/books/{bookId}/cover`. Clients resolve it against the server address
and send their bearer token, so uploaded covers are never public. `DELETE`
removes it. Covers from other sites keep their absolute `https` URL.

`POST /books` accepts `multipart/form-data` with exactly one `file` part and an
optional `title` field. The server validates the file contents rather than its
extension and preserves the original bytes. A MOBI, AZW, or AZW3 file is
converted to EPUB with Calibre's `ebook-convert` and stored as that EPUB; a
server without Calibre refuses it with `415 converter_missing`. Any other
format returns `415 unsupported_book_format`. From an EPUB it also reads the
title, authors, description (as plain text), series (Calibre's
`calibre:series`/`series_index` or an EPUB 3 `belongs-to-collection`), and
embedded cover. A file byte-identical to an existing edition, here or through
`POST /books/{bookId}/editions`, returns `409 duplicate_book` naming the book
it matches. The default upload limit is 512 MiB and can be changed with
`BOOKHARBOR_MAX_UPLOAD_BYTES`.

An edition response includes format, byte length, media type, SHA-256 checksum,
and an authenticated content URL. A book built from a followed web novel also
has `webnovelChapters`, the chapters in it so far; when its file is replaced
with more chapters, the edition keeps its ID and gets a new checksum, so
clients know to download it again. Content supports `GET`, `HEAD`, and standard
byte ranges so Android can verify and resume a partial download.

`PATCH /books/{bookId}` accepts any non-empty subset of `title`, `subtitle`,
`description`, `authors`, `coverUrl`, `series`, `seriesIndex` (0 for
unnumbered), `tags`, and a reviewed external metadata `source`:

```json
{
  "title": "The Left Hand of Darkness",
  "subtitle": "A Novel",
  "authors": ["Ursula K. Le Guin"],
  "description": "...",
  "coverUrl": "https://...",
  "series": "Hainish Cycle",
  "seriesIndex": 4,
  "tags": ["Science fiction", "Book club"],
  "source": { "provider": "hardcover", "id": "12345" }
}
```

Metadata search is a suggestion workflow rather than a direct write. When
`BOOKHARBOR_HARDCOVER_TOKEN` is configured, the admin search endpoint queries
Hardcover on the server and returns provider-neutral candidates. The
administrator reviews a candidate and applies its fields through the ordinary
book patch route. Provider credentials are never exposed to the browser.

## Book requests

```text
GET    /metadata/search?q=
GET    /book-requests
POST   /book-requests
DELETE /book-requests/{requestId}
GET    /admin/book-requests?status=open|resolved   administrator only
POST   /admin/book-requests/{requestId}/fulfill    administrator only
POST   /admin/book-requests/{requestId}/decline    administrator only
```

`GET /metadata/search` is open to every reader for finding a book to request.
It returns candidates as `{ "provider", "id", "title", "subtitle", "authors",
"coverUrl", ... }`. With web novels available, novelarchive.cc matches follow
the books, with provider `novelarchive` and a subtitle such as
`Web novel · 3188 chapters · ongoing`.

`POST /book-requests` takes `{ "title", "author", "coverUrl", "sourceProvider",
"sourceId" }`; the source identifies the candidate so requests for the same
book can be grouped. A reader lists and cancels only their own requests.

Fulfilling a request requires `{ "bookId" }` naming a book already in the
library, so approval always means the book is there. A request for a web novel
is fulfilled automatically once the followed novel's book exists.

## Lists

```text
GET    /lists
POST   /lists
PATCH  /lists/{listId}
DELETE /lists/{listId}
GET    /lists/{listId}/books
POST   /lists/{listId}/books
DELETE /lists/{listId}/books/{bookId}
```

Lists belong to the reader who made them; every route acts only on the
caller's own lists. `GET /lists` includes each list's `bookIds`, newest first.
Creating or renaming takes `{ "name" }` (1 to 200 characters); a name the
reader already uses, ignoring case, returns `409 list_name_taken`. Adding
takes `{ "bookId" }`; adding a book already in the list does nothing.

## Friends

```text
GET    /friends
GET    /friends/{userId}
DELETE /friends/{userId}
GET    /friends/requests
POST   /friends/requests
POST   /friends/requests/{userId}/accept
DELETE /friends/requests/{userId}
GET    /me/social-settings
PUT    /me/social-settings
```

A friend request is sent by email address (`{ "email" }`). `GET
/friends/requests` returns `{ "incoming", "outgoing" }`; deleting a request
declines one sent to you or cancels one you sent.

Reading activity is private unless a reader turns it on:
`PUT /me/social-settings` takes `{ "activityVisible", "goalYear", "goalBooks" }`
and both routes return those plus `finishedThisYear`. `GET /friends` lists
accepted friends with what they are currently reading, their books finished
this year, and their goal, where they share activity. `GET /friends/{userId}`
is a friend's page: their reading now, books finished, and books in common.
Anyone who is not an accepted friend gets `404`.

## Shelfmark

A [Shelfmark](https://github.com/calibrain/shelfmark) instance, once set up in
Settings, is a source for fulfilling requests.

`GET /admin/shelfmark/search?title=&author=` searches every enabled Shelfmark
source and returns `{ "items": [{ "source", "sourceId", "title", "format",
"language", "size", "indexer", "raw" }] }`.

`POST /admin/shelfmark/downloads` takes `{ "release": <an item's raw>,
"requestIds": [...] }` and queues it on Shelfmark (`202`). The server follows
the download, imports the file when it finishes (converting MOBI/AZW3), and
fulfills the given requests with the book. `GET /admin/shelfmark/downloads`
lists the downloads being followed, newest first, each with `status` (Shelfmark's
own, then `importing`, `imported`, or `failed`), `progress`, `message`, and
`bookId` once imported. This list is kept in memory and resets when the server
restarts; the files themselves stay in Shelfmark.

## Web novels

`GET /admin/webnovels/search?q=` searches novelarchive.cc and marks novels
already followed.

`POST /admin/webnovels` takes `{ "sourceId" }` and follows the novel (`202`;
`200` if it was already followed, which also fulfills its open requests when
its book exists). A background worker then fetches its chapters, about two a
second, and builds them into an EPUB:

- The book is added to the library once the first 100 chapters are in and is
  rebuilt every 100 chapters after that, then once more at the end.
- Every chapter is listed from the start. Chapters not fetched yet are short
  placeholders under their real names, and chapter N always stays at position
  N, so reading positions hold as placeholders fill in.
- Ongoing novels are checked again every `webnovels.syncHours`, and only new
  chapters are fetched. Completed novels are not checked again.
- If the site stops answering its API, the sync stops with an error and
  retries an hour later. Chapters already fetched are kept.

`GET /admin/webnovels` returns the followed novels, each with `bookId`,
`chapters`, `checkedAt`, `error`, and `progress` (what the worker is doing with
it now), plus `intervalHours`. `POST /admin/webnovels/{sourceId}/sync` checks
a novel now, whatever its schedule. `DELETE /admin/webnovels/{sourceId}` stops
following it; its book stays in the library.

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
