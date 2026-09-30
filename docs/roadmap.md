# Feature roadmap

This records the requested priorities and the first implementation slice.
“Pending” means the requested capability is not complete, even where existing
code provides a partial foundation.

| Priority | Capability | Status |
|---|---|---|
| P1 | Server-side catalog search | Implemented API, admin UI, and Android Browse: metadata substring search, format/tag/series/library filters, matching counts, cursor pagination, and cached offline search. Android device testing pending. |
| P1 | Saved filters | Implemented private per-user persistence and API; admin and Android create/apply/update/delete. Android device testing pending. |
| P1 | Named libraries with user access controls | Implemented API and admin UI: per-library reader grants, default library migration, book assignment, and access checks across catalog/content/sync/social paths. Android library chooser implemented; device testing pending. |
| P1 | OPDS for external readers | Implemented OPDS 1.2: library/saved-filter navigation, search, paginated feeds, covers, acquisition, and revocable per-user credentials. Real external-reader validation pending. |
| P2 | Scanner exclusions and file-type controls | Implemented for watched sources: relative exclusion patterns, EPUB/PDF selection, admin controls, and scan reconciliation. |
| P2 | Per-source schedules | Implemented: each watched source can use the server default or its own interval; startup and manual scans remain available. |
| P2 | Richer metadata and metadata locks | Implemented EPUB publisher/date/language/ISBN, subtitle/subjects, field origins and locks, admin edits and EPUB refresh; rescans preserve locks and reading IDs. PDF embedded metadata and sidecars remain pending. |
| P2 | PDF page flipping and two-page layouts | Pending |
| P2 | Uploaded fonts and per-book reading profiles | Pending |
| P2 | Full vertical/RTL EPUB support | Pending |
| P2 | Link navigation history and next-book navigation | Pending |
| P2 | Centralized annotation management | Pending; per-book annotations and sync exist. |
| P2 | Ordered reading lists and collections | Pending; private unordered lists exist. |
| P2 | Want to Read and personal ratings | Pending |
| P2 | Granular permissions | Pending; administrator/reader roles exist. |
| Later | Browser reader | Pending |
| Later | SSO/OIDC | Pending |
| Later | Customizable dashboards | Pending |
| Later | Comic, manga, and image readers | Pending |

## Next implementation boundary

Android adoption of server search, saved filters, and the library chooser is
implemented. Device testing and external OPDS reader interoperability remain
validation work. Library access is enforced by the server for all clients.

The first P2 metadata slice is implemented with explicit field locks and
provenance. Automatic EPUB metadata refresh preserves manual edits and reading
identities. PDF embedded metadata and optional sidecars remain future work.
Scanner controls and per-source intervals are implemented.

Search currently scans metadata for literal substring matches. Large-catalog
profiling can guide adding a search index while preserving the documented
matching behavior.
