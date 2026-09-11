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
never credential data.

## Library

```text
GET    /books?limit=
GET    /books/{bookId}
POST   /books                         administrator only
PATCH  /books/{bookId}                administrator only
POST   /books/{bookId}/editions       planned; administrator only
GET    /editions/{editionId}/content
```

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
