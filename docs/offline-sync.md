# Offline reading-progress synchronization

Reading and saving a position never depend on a live server. The Android client
owns a local progress record, a durable outbox of immutable events, and a cursor
for server changes. The server accepts retries safely and keeps the latest
reading activity from every device as the canonical position.

## Local write path

Whenever the reader records a meaningful location, one local database
transaction must:

1. update the displayed local position;
2. insert an outbox event with a new stable event ID; and
3. retain the event's device, book, edition, locator, percentage, and occurrence
   time exactly as first written.

The UI may acknowledge the save only after that transaction commits. The
outbox survives process death, device restarts, airplane mode, and a server
outage. Opening a downloaded book and saving new positions continue normally
while disconnected.

## Exchange loop

Android schedules one unique background sync job using WorkManager. The job
requires network connectivity, sends up to 100 oldest outbox events to
`POST /api/v1/progress/sync`, and includes the last committed server cursor.

After a successful response, one local transaction:

1. removes only outbox events explicitly acknowledged by event ID;
2. merges returned canonical positions into the local cache;
3. records the returned cursor; and
4. schedules the next page immediately when `hasMore` is true.

If the request times out, the response is lost, authentication renewal fails,
or the server is unavailable, none of those local records are removed. The job
uses exponential backoff and retries the same event IDs. A server that already
committed the first attempt returns the original revisions as duplicates.

## Reconciliation

The server assigns every accepted event a monotonically increasing revision.
For each reader and book, the event with the latest `occurredAt` becomes
canonical; the higher server revision wins when timestamps are equal. Older
events arriving late are acknowledged as `superseded` and kept for audit, but
never move the current position backward.

The client applies a server position when its source event is newer than the
local position and is not still represented by an unacknowledged local event.
This keeps freshly recorded offline activity visible until the server has had a
chance to reconcile it. Server times more than five minutes in the future are
rejected; the client should surface a device-clock problem rather than rewrite
the original event.

## Required invariants

- The same event ID always carries the same immutable payload.
- Cursor and outbox acknowledgement changes commit atomically.
- A failed sync never deletes queued work.
- Progress percentages are display summaries; format-specific locators reopen
  books.
- Download, reader, and local progress paths contain no synchronous network
  dependency.
- Logout either finishes sync first or clearly warns before deleting a user's
  unacknowledged local events.
