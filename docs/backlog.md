# Open items from the pre-v1 UI/UX and feature audit

Recorded 2026-09-22, after a UI/UX and feature-completeness audit against
`product.md`, `reader.md`, and `architecture.md`. The audit's concrete findings
(non-functional reader style options, the dead-end chapter-finish button,
missing quick chapter/page navigation, unlabeled activity-log actions, and the
always-shown "Find online" tab) have been fixed. The direction items below
have since been implemented too (see the note under each); the audit coverage
gaps remain open.

## Direction items (resolved 2026-09-22)

### No restore path for backups

Fixed: `bookharbor restore [--force] <archive.zip>` (`apps/server/cmd/bookharbor/main.go`,
`apps/server/internal/export/restore.go`) is an offline CLI operation, not a
live API -- it refuses to run against a data directory that already has a
database unless `--force` is passed, since it's a wholesale replace of the
database and book/cover files, not a merge. `apps/admin/src/views/backup.tsx`
now points to the command.

### No reading stats for the reader themselves

Fixed: `GET`/`PUT /api/v1/me/social-settings` now also returns
`finishedThisYear`, computed the same way as the friends endpoint
(`s.reading.FinishedCount`, `apps/server/internal/httpapi/friends.go`). The
Android Friends tab shows it as a "Your reading" section with goal progress
(`apps/android/.../library/FriendsScreen.kt`).

### Docs vs. shipped scope have drifted

Fixed: `product.md`'s deferred list and
[`decisions/0001-mvp-before-social.md`](decisions/0001-mvp-before-social.md)
have been updated to reflect that friends, feeds, and reading goals shipped
with full UI.

## Requested features (recorded 2026-09-22)

### Library grid view -- resolved

`LibraryTab` now switches between list and grid via a toggle next to search
(`LibraryViewMode`, `apps/android/.../library/LibraryScreen.kt`), backed by
`LazyVerticalGrid`. No server changes needed.

### Book requests via Hardcover search -- resolved

A reader searches (`GET /api/v1/metadata/search`, any authenticated user) and
files a request (`book_requests` table, `internal/requests`); an admin works
a queue (`GET /api/v1/admin/book-requests`) and can only fulfill a request by
pointing it at a book that already exists in the library
(`POST .../{id}/fulfill` checks `library.Get` first and rejects with
`book_not_found` otherwise) -- approval always means "uploaded and linked,"
never a bare status flip. Admin UI: `apps/admin/src/views/requests.tsx`.
Android UI: a "Requests" tab (`apps/android/.../library/RequestsScreen.kt`).

### Custom lists -- resolved

A reader can group books into their own named lists, separate from the fixed
`ShelfFilter` categories. Server: `book_lists`/`book_list_items` tables,
`internal/lists`, CRUD under `/api/v1/lists` -- every operation is scoped to
the caller's own lists (ownership is checked in the store, not just the
route), and adding a book already in a list is a no-op rather than an error.
Android: "Your lists" from the Library tab (`ListsScreen.kt`: an index of
lists and each list's contents), and "Add to list" in each book's "⋮" menu.
No admin UI -- like friends and reading stats, this is a private,
reader-owned feature.

## Audit coverage gaps (done 2026-09-22)

The 2026-09-22 audit was scoped to UI/UX and feature completeness only, at
standard depth (hotspot-weighted, not exhaustive) -- it explicitly skipped
the four items below. Each has now had a pass, scoped to the changes made in
this same session (the export/restore, book requests, lists, and reader
chrome features); none of these are full whole-codebase audits, so a wider
pass is still worth doing before v1 if the areas they didn't touch matter
(e.g. the older friends/sync/reader code, which none of these re-examined).

### Security review

Found and fixed one real, high-severity issue: a zip-slip path-traversal bug
in `bookharbor restore` (`apps/server/internal/export/restore.go`). The
`storage_path` values it reads back out of the archive's own (untrusted)
`bookharbor.db`, and the `covers/<bookID>` zip entry names, were joined
straight into filesystem paths with no validation -- a crafted archive with
`storage_path = "../../../../home/user/.ssh/authorized_keys"`, or a zip entry
literally named `covers/../../etc/cron.d/evil`, could write outside the
target data directory when an admin ran `restore` on a tampered or
untrusted `.zip`. `export.go` already validates this exact thing when
*writing* an archive; `restore.go` didn't mirror it. Fixed by applying the
same `books/` prefix check on read, and rejecting any `covers/` entry whose
book ID contains a path separator. Regression tests
(`TestRestoreRejectsPathTraversal`) confirm both sinks are closed, and were
verified to fail against the pre-fix code before the fix landed. Everything
else examined (list/request ownership scoping, the new reader-facing
metadata-search endpoint, admin route gating) checked out clean.

### Accessibility pass

Found and fixed one real issue in the reader chrome auto-hide feature added
this session: hiding the top/bottom bars on scroll relied on a plain
`pointerInput`/`detectTapGestures` middle-tap to bring them back, which
TalkBack's touch-exploration mode generally can't trigger the normal way --
a TalkBack user could lose the close/contents/settings buttons with no
reliable way back (short of the system Back gesture, which still worked).
Fixed by tracking `AccessibilityManager.isTouchExplorationEnabled` live
(`rememberTouchExplorationEnabled`, `apps/android/.../reader/ReaderChrome.kt`)
and never auto-hiding, and forcing the chrome back visible, whenever it's on.
Also found the admin's new "Fulfill" search input
(`apps/admin/src/views/requests.tsx`) skipped the sr-only `<label>` the
codebase's own search-input convention uses (see `library.tsx`'s book
search) -- fixed to match. `contentDescription` coverage on the rest of the
session's new Android screens (Lists, Requests) was spot-checked and already
correct.

### Dependency audit

Go: `govulncheck` reports 0 vulnerabilities reachable from this code (one
advisory in `golang.org/x/crypto/openpgp`, a package this codebase never
imports). Admin: `npm audit` reports 0 vulnerabilities. Android: no local
vulnerability-database tool available to check `org.jsoup:jsoup:1.18.1`
precisely; it's a reasonably current version and jsoup's HTML parser doesn't
resolve external entities, so XXE-class risk is low, but this one dependency
wasn't verified against a live CVE database and is worth a follow-up check.

### Performance review

No regressions found. The new list/request endpoints do fetch related rows
in a loop (`s.library.Get` per book in a list, `s.users.GetUser` per open
request) rather than batching -- an N+1 pattern, but it's the same pattern
`friends.go` already uses for the same shape of problem, consistent with
this codebase's existing tradeoff of simplicity over batching at this app's
scale (a single self-hosted server, bounded per-user list sizes). New tables
(`book_requests`, `book_lists`, `book_list_items`) have indexes on every
column they're filtered or joined by. No Android main-thread work was added
outside the existing `Dispatchers.IO` convention.
