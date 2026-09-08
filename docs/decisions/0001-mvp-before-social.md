# 0001: Complete the reading loop before social features

- Status: accepted
- Date: 2026-09-21

## Decision

The first release will implement importing, browsing, offline reading, and
progress synchronization before friends, activity feeds, or reading goals.

## Reason

Every social feature depends on trustworthy identity, book identity, and
reading progress. Building those foundations through an actual offline reading
loop exposes format, synchronization, privacy, and content-delivery problems
early. It also produces a useful private library even if social work is delayed.

## Consequences

- Social visibility is considered in ownership rules but has no version 0.1 UI.
- Progress remains private by default.
- The server architecture stays modular enough to add relationships and activity
  without placing those concerns inside the Reader or Library modules.
