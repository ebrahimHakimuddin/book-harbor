# BookHarbor documentation

- [`product.md`](product.md) defines the first release and its acceptance
  criteria.
- [`architecture.md`](architecture.md) defines the initial modules and the
  seams between them.
- [`api.md`](api.md) sketches the first client/server interface.
- [`offline-sync.md`](offline-sync.md) defines lossless reading-progress
  synchronization through client or server outages.
- [`reader.md`](reader.md) defines EPUB/PDF navigation, progress display, and
  offline reader customization.
- [`decisions/0001-mvp-before-social.md`](decisions/0001-mvp-before-social.md)
  records why the reading loop comes before social features.
- [`decisions/0002-go-server-kotlin-android.md`](decisions/0002-go-server-kotlin-android.md)
  records the initial implementation stack.
- [`decisions/0003-sqlite-metadata-store.md`](decisions/0003-sqlite-metadata-store.md)
  records the default metadata store.

These documents describe intended behavior. Once implementation begins, tested
code and the versioned HTTP contract become authoritative where they differ.
