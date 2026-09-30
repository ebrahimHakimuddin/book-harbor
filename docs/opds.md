# External readers with OPDS

BookHarbor serves an authenticated [OPDS 1.2 catalog](https://specs.opds.io/opds-1.2)
at `/opds`. It supports navigation by named library or private saved filter,
metadata search, cursor pagination, book entries, local covers, and direct
EPUB/PDF acquisition. Managed disk files, object storage, and mounted editions
use the same download handler as the Android app, including HTTP ranges.

## Connect a reader

1. Open **Readers** in the admin console and select **External reader access**
   for the account that will use the reader app.
2. Give the connection a name, such as “KOReader on my tablet,” and create it.
3. In the reader app, add an OPDS catalog with the displayed catalog URL,
   username, and password. The password is returned only when created; save it
   in the reader app before closing the dialog.

Use the server's HTTPS URL when connecting over the internet. Authentication
uses HTTP Basic with the generated connection username and password. The
account's email/password and ordinary API session tokens do not authenticate
OPDS. Credentials are sent in the Authorization header, never in catalog URLs.

Revoke a connection from the same dialog to stop subsequent catalog and download
requests. Each account may have up to 20 connections. Disabling/deleting an
account or changing/resetting its password revokes its OPDS connections.
Re-enabling an account requires new connections. Backups omit OPDS credentials.

## Library access

Every request checks current account and library access. A saved filter never
grants access. Revoked library grants apply to feeds, individual entries,
covers, and acquisition links immediately on the next request. Administrators
can access every library. Downloaded copies remain on the external device.

Watched files marked unavailable are omitted from acquisition feeds. A mount
that goes offline between indexing and acquisition returns `503`; the catalog
identity is retained. Local cover images are supported; remote metadata cover
URLs are omitted to keep all authenticated catalog links on this server.

## API and routes

Signed-in users manage their own credentials through:

```text
GET    /api/v1/me/opds-credentials
POST   /api/v1/me/opds-credentials          { "name": "Reader app" }
DELETE /api/v1/me/opds-credentials/{id}
```

Administrators use `/api/v1/admin/opds-credentials` with `?userId={userId}` for
the same operations. Creation returns `credential`, `username`, `password`,
and `catalogUrl`; listing returns only credential IDs, names, creation times,
and the catalog path. Stored secrets are SHA-256 hashes of random 256-bit keys.
OPDS credentials authorize only GET/HEAD requests under `/opds`.

```text
GET /opds                            Navigation root
GET /opds/books                      Paginated acquisition feed
GET /opds/search.xml                 OpenSearch description
GET /opds/books/{id}                 Full Atom book entry
GET /opds/books/{id}/cover           Local cover
GET /opds/editions/{id}/content      Book download
```

The acquisition feed accepts the same `q`, `format`, `tag`, `series`, `libraryId`,
`limit`, and `cursor` parameters as `/api/v1/books`. Follow the feed's `next`
link for pagination. Feed links are relative to the server; the OpenSearch
template uses the request host and HTTPS when TLS or `X-Forwarded-Proto: https`
is present. Configure the reverse proxy to preserve the public Host and scheme.

Automated checks cover XML parsing and escaping, OpenSearch discovery,
pagination, permissions, credentials, backup scrubbing, range requests, and
mounted-source downloads. Actual KOReader/Librera device interoperability still
needs validation. OPDS does not synchronize progress or annotations with those
apps, and OPDS 2.0/OPDS-PS are not implemented.
