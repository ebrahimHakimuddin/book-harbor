# Feature parity and pending work

Updated 2026-09-30. This replaces the completed implementation plans. The agreed completion scope is the **original feedback**: existing NAS libraries, page turns, formatting, background colors/images, and a warm reading filter. Broader Kavita parity remains a future roadmap.

“Implemented” means code exists and automated checks passed; it does not mean the running server was deployed or the APK was tested on a physical device.

## Original feedback: implementation status

| Request | Delivered | Remaining validation / limits |
| --- | --- | --- |
| Index an existing NAS library without reimporting | Server indexes mounted EPUB/PDF folders in place, preserving source files; managed imports remain available. | Operator mounts SMB/NFS on the host/container. BookHarbor does not mount shares or store NAS credentials. Real NAS deployment still needs validation. |
| Keep the catalog updated | Startup, periodic and manual scans; source management and per-file errors in admin Settings. Rename/replacement reconciliation, missing-file grace scans and offline-source preservation. Per-source exclusion patterns, EPUB/PDF selection, and scan intervals are available. | Polling, not filesystem event watching. |
| Android access, offline reading and sync | Watched editions use the existing authenticated catalog/content API, verified downloads and progress/annotation sync. | User will test the NAS-to-APK workflow. The app downloads book bytes for offline reading. |
| Flip pages | EPUB scroll, one-page and wide-screen two-page modes, measured pagination, passage restoration and chapter transitions. | User device testing pending. PDF remains a continuous page list. Vertical/RTL EPUB layouts fall back to scroll. |
| Formatting | Existing font, size, spacing, margin, alignment and hyphenation controls work with EPUB page modes. | Device-local profile; uploaded fonts and per-book profiles remain pending. |
| Background colors/images | Validated solid colors and privately stored, bounded background images, shared reader settings. | User device testing pending; image/text readability needs review with actual books. |
| Blue-light comfort | Adjustable warm tint within the reader. | This is a reading surface tint, not a device-wide blue-light filter. |
| Readable connection errors | API errors sanitize intermediary JSON/HTML; Friends offers retry. Cloudflare 530 regression covered. | This fixes the displayed error; the operator still needs to restore an unavailable tunnel. |

Full backups include watched bytes and restore them into owned storage. Reference backups retain external paths and omit watched bytes; restore requires those mounts. Disabled sources remain eligible for full backup. See [setup and backups](../README.md), [API](api.md), [architecture](architecture.md), and [reader behavior](reader.md).

## Verification and pending release work

- Server: `go test ./...` and `go vet ./...` passed, including source CRUD, scan reconciliation, unavailable mounts, source-byte preservation, backups/restores, root symlink rejection and re-enable overlap checks.
- Admin: production build and lint passed; embedded assets regenerated. Existing lint warnings remain.
- Android: all 90 unit tests passed; release APK built and signature verified. Gradle was stopped afterward.
- User testing pending: install `apps/android/app/build/outputs/apk/release/app-release.apk`; check page turns/reflow, custom backgrounds/warmth, PDF behavior, offline reading, resume and sync with real books. Device installation/testing belongs to the user; no ADB checks are required.
- Deployment pending: mount the NAS read-only, configure allowed roots, start the updated server, then exercise add/replace/rename/remove and reconnect scenarios. Commits do not deploy the server.
- Current catalog, OPDS, scanner and metadata changes remain in the working tree for review.

## Kavita comparison

BookHarbor status reflects the current working tree. Kavita's libraries, EPUB reader, OPDS and filtering documentation were rechecked on 2026-09-30; other rows retain the official-documentation survey from 2026-09-29. This is a scoped capability comparison, not a claim of complete parity.

| Capability | BookHarbor | Kavita | Status / next step |
| --- | --- | --- | --- |
| Existing folders / mounted NAS | EPUB/PDF in-place indexing, multiple managed sources | Library folders on mounted storage | Core workflow implemented; not full library-management parity. [Libraries](https://wiki.kavitareader.com/guides/admin-settings/libraries/) |
| Automatic scans | Startup, periodic polling, manual scan and verification; per-source exclusions, format selection, and scan intervals | Scheduled scans and optional delayed folder watching | Partial; event watching pending. [Libraries](https://wiki.kavitareader.com/guides/admin-settings/libraries/), [tasks](https://wiki.kavitareader.com/guides/admin-settings/tasks/) |
| EPUB page modes | Scroll, one-page, two-page in Android | Scroll and column/page layouts in browser | Core modes implemented; advanced layout support pending. [EPUB reader](https://wiki.kavitareader.com/guides/readers/epub/) |
| Reader appearance | Themes, typography, custom color/image, warmth, brightness | Themes, typography, custom fonts and reading profiles | Partial; uploaded fonts and per-book/library profiles pending. [EPUB reader](https://wiki.kavitareader.com/guides/readers/epub/), [profiles](https://wiki.kavitareader.com/guides/user-settings/reading-profiles/) |
| Offline Android reading | Native verified downloads; progress and annotation sync | Web readers and third-party clients | Different client approach; test the chosen NAS workflow. [Clients](https://wiki.kavitareader.com/guides/3rdparty/kavita-dedicated/) |
| Search and saved filters | Server-side metadata queries, format/tag/series filters, cursor pagination, and private saved filters in admin and Android Browse | Metadata queries and saved Smart Filters | Implemented baseline; Android device testing pending. [Filtering](https://wiki.kavitareader.com/guides/features/filtering/) |
| OPDS / independent readers | OPDS 1.2 navigation/search/acquisition with revocable credentials and live library access | OPDS, OPDS-PS and external integrations | Implemented baseline; device interoperability validation and OPDS-PS remain. See [BookHarbor OPDS](opds.md). [OPDS](https://wiki.kavitareader.com/guides/features/opds/) |
| Named libraries and user access | Logical catalog libraries with reader grants, default-library migration, and book assignment | Named libraries with multiple roots and user access | Implemented access baseline and Android library chooser; device testing pending. [Libraries](https://wiki.kavitareader.com/guides/admin-settings/libraries/), [users](https://wiki.kavitareader.com/guides/admin-settings/users/) |
| Browser reading | Admin console; reading is Android-only | Browser EPUB/PDF/comic readers | Missing. [EPUB](https://wiki.kavitareader.com/guides/readers/epub/), [PDF](https://wiki.kavitareader.com/guides/readers/pdf/) |
| Metadata | Rich EPUB extraction, manual edits, Hardcover suggestions, field locks/origins and EPUB refresh; filename/folder PDF fallback | Broader OPF/XMP fields and parsing | EPUB baseline implemented; embedded PDF metadata and sidecars pending. [EPUB metadata](https://wiki.kavitareader.com/guides/metadata/epubs/), [PDF metadata](https://wiki.kavitareader.com/guides/metadata/pdfs/) |
| Collections / lists | Private book lists, requests and social features | Collections, ordered reading lists, CBL import | Partial; ordering, collections, Want to Read and personal ratings pending. [Lists](https://wiki.kavitareader.com/guides/features/readinglists/), [collections](https://wiki.kavitareader.com/guides/features/collections/) |
| Reader navigation / annotations | TOC, internal links, bookmarks, highlights, notes, search, TTS and text export | Browser annotation controls, link history and next-item navigation | Partial; link back stack, next-book navigation and annotation management pending. [EPUB](https://wiki.kavitareader.com/guides/readers/epub/) |
| PDF layouts | Vertical page list with zoom | Multiple reading layouts | Partial; horizontal and two-page PDF modes pending. [PDF](https://wiki.kavitareader.com/guides/readers/pdf/) |
| Permissions / SSO | Admin/reader roles and disabled users | Library/user permissions, age access and OIDC | Partial; granular controls and OIDC pending. [Users](https://wiki.kavitareader.com/guides/admin-settings/users/), [OIDC](https://wiki.kavitareader.com/guides/admin-settings/open-id-connect/) |
| Dashboard customization | Fixed home shelves and widget | Custom dashboard/navigation and filter streams | Partial. [Customization](https://wiki.kavitareader.com/guides/features/customization/) |
| Comics / manga / image libraries | No native comic catalog/reader; EPUB/PDF only | Dedicated types/readers | Missing. [Libraries](https://wiki.kavitareader.com/guides/admin-settings/libraries/), [comic reader](https://wiki.kavitareader.com/guides/readers/comic-manga/) |

MOBI/AZW/AZW3 managed imports can be converted to EPUB; they are not supported as in-place watched editions. Kavita+ external metadata, scrobbling, recommendations and smart collections are separate subscription-related product decisions. See [Kavita+](https://wiki.kavitareader.com/kavita%2B/metadata-controls/).

## Prioritized future work

These are pending features, outside the agreed original-feedback completion target.

1. **P1 — Catalog scale and interoperability:** server-side search, saved filters, permission-scoped libraries, and OPDS credentials/feeds are implemented on the server, admin, and Android Browse. Android device testing and real Librera/KOReader interoperability validation remain.
2. **P2 — Scanner and metadata controls:** exclusions, file-type controls, and per-source schedules are implemented. Rich EPUB metadata, provenance/locks and protected refresh are implemented. Embedded PDF metadata and optional sidecars remain.
3. **P2 — Reading:** PDF page modes, custom fonts, per-book/library profiles, EPUB link history, next-book/list-item transitions and annotation management. Browser reading requires a separate client project if desktop reading becomes a target.
4. **P2 — Personal catalog:** ordered lists, collections, Want to Read, ratings and granular user/download/age permissions.
5. **P3 — Wider product scope:** OIDC, configurable dashboards, CBZ/CBR/image libraries and dedicated comic/manga readers. Priorities should change if those readers become the target audience.
