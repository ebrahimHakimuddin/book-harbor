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

## Library

```text
GET    /books?cursor=&query=
GET    /books/{bookId}
POST   /books                         administrator only
PATCH  /books/{bookId}                administrator only
POST   /books/{bookId}/editions       administrator only
GET    /editions/{editionId}/content
```

An edition response includes format, byte length, media type, checksum, and a
content revision. Content supports standard byte ranges so Android can resume a
partial download.

## Reading progress

```text
GET    /progress?changedAfter=&cursor=
GET    /books/{bookId}/progress
PUT    /books/{bookId}/progress
```

A progress write contains:

```json
{
  "editionId": "edition_opaque_id",
  "deviceId": "device_opaque_id",
  "baseRevision": 12,
  "occurredAt": "2026-09-21T12:00:00Z",
  "locator": {
    "kind": "epub-cfi",
    "value": "epubcfi(...)"
  },
  "percentage": 0.42
}
```

PDF uses a tagged locator such as `{ "kind": "pdf-page", "page": 84 }`.
The server returns a monotonically increasing revision. A stale `baseRevision`
produces a conflict response containing current server state; the client must
make reconciliation visible rather than silently overwriting newer progress.

## Contract rules

- Timestamps are UTC RFC 3339 values and are not used as the sole conflict
  authority because client clocks are untrusted.
- Percentages range from `0` through `1`; they are summaries, not reopen
  locations.
- Unknown response fields are ignored by clients.
- Breaking changes require a new major route prefix.
- Original-file checksums use an explicitly named algorithm, initially SHA-256.
