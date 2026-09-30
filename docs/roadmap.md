# Feature roadmap

This records the requested priorities and the first implementation slice.
“Pending” means the requested capability is not complete, even where existing
code provides a partial foundation.

| Priority | Capability | Status |
|---|---|---|
| P1 | Server-side catalog search | Implemented API and admin UI: metadata substring search, format/tag/series filters, matching counts, cursor pagination. Android UI adoption pending. |
| P1 | Saved filters | Implemented private per-user persistence and API; admin create/apply/update/delete. Android UI adoption pending. |
| P1 | Named libraries with user access controls | Pending |
| P1 | OPDS for external readers | Pending |
| P2 | Scanner exclusions and file-type controls | Pending |
| P2 | Per-source schedules | Pending |
| P2 | Richer metadata and metadata locks | Pending |
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

Named libraries should be logical catalog groups, distinct from watched disk
sources and private reading lists. Access decisions must cover catalog search,
book detail, covers, downloads, list membership, and any API that reveals book
metadata. Administrators need a migration-compatible default library so existing
instances retain their catalog and reader access.

OPDS should follow the library access layer and use the same availability and
permission checks for feeds and acquisition links. Decide external-reader
credentials and their revocation before adding feed endpoints.

Search currently scans metadata for literal substring matches. Large-catalog
profiling can guide adding a search index while preserving the documented
matching behavior.
