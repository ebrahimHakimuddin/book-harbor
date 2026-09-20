# Open items from the pre-v1 UI/UX and feature audit

Recorded 2026-09-22, after a UI/UX and feature-completeness audit against
`product.md`, `reader.md`, and `architecture.md`. The audit's concrete findings
(non-functional reader style options, the dead-end chapter-finish button,
missing quick chapter/page navigation, unlabeled activity-log actions, and the
always-shown "Find online" tab) have been fixed. The items below were raised
but not acted on, and the audit's own scope had gaps; both are listed here so
they aren't lost.

## Direction items

### No restore path for backups

The admin "Export everything" feature (`apps/admin/src/views/backup.tsx`,
`apps/server/internal/export`) only exports. There is no way to bring a
produced `.zip` back into a server. For a self-hosted personal library, a
backup that cannot be restored is the gap that hurts most exactly when it is
needed. Likely bigger than export: needs data-merge/overwrite semantics for
an existing database and file directory, and is probably safer as an
offline/CLI operation than a live-API one (it may be restoring the same
database the server is currently using).

### No reading stats for the reader themselves

The friends feature already computes "finished this year" and annual goals
for *friends* (`apps/server/internal/httpapi/friends.go`), but a reader has
no view of their own lifetime reading stats anywhere -- that data already
exists server-side per reader. A self-facing view (books finished this year,
goal progress) would reuse the same `reading.FinishedCount` the friends
endpoint already calls.

### Docs vs. shipped scope have drifted

`product.md`'s "Explicitly deferred" list and
[`decisions/0001-mvp-before-social.md`](decisions/0001-mvp-before-social.md)
(dated 2026-09-21, "no version 0.1 UI" for friends) both predate the friends
feature shipping with full UI. Not a code defect -- but misleading for future
contributors (or agents) reading these docs about what is actually in scope.
Update `product.md`'s deferred list and add a follow-up note to ADR 0001
reflecting that friends shipped ahead of the reading loop's full
"dependability," or record a new ADR explaining why the sequencing changed.

## Audit coverage gaps

The 2026-09-22 audit was scoped to UI/UX and feature completeness only, at
standard depth (hotspot-weighted, not exhaustive). It did not cover:

- Security review
- Performance review
- A manual accessibility/TalkBack pass (only `contentDescription` coverage
  was spot-checked, and looked solid)
- Dependency audit or server-side architecture/tech-debt review

Any of these would need a separate pass before being confident v1 is ready
on those axes.
