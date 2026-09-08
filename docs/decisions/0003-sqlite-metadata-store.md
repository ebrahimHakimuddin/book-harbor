# 0003: Use SQLite as the default metadata store

- Status: accepted
- Date: 2026-09-21

## Decision

BookHarbor will store instance, identity, library, and reading metadata in a
SQLite database inside the configured data directory. Connections enable WAL,
foreign-key enforcement, a bounded busy timeout, normal synchronization, and
SQLite defensive mode.

## Reason

SQLite keeps the default self-hosted deployment to one process and one durable
directory. BookHarbor's initial workload is a good match for a transactional
embedded database: many reads, relatively small metadata, and modest write
concurrency. Backing up original files together with one metadata database is
also straightforward for a home-server operator.

## Consequences

- One BookHarbor server process owns a data directory at a time.
- Database access uses a bounded connection pool and short transactions.
- Schema changes are versioned and applied during startup.
- A future external database adapter requires demonstrated operational demand;
  it is not an abstraction added preemptively.
