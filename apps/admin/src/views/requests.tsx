import { useEffect, useMemo, useRef, useState, type FormEvent } from "react"
import { BookDownIcon, BookMarkedIcon, CheckCircle2Icon, CopyIcon, DownloadIcon, HistoryIcon, InboxIcon, Loader2Icon, SearchIcon, UploadIcon, UsersIcon, XIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, APIError, type Book, type BookRequest } from "@/lib/api"
import { keys, useBookRequests, useBooks, useDeclineBookRequest, useFulfillBookRequest, useImportBook, useUpdateBook } from "@/lib/queries"
import { errorMessage, timeAgo } from "@/lib/format"
import { Cover } from "@/components/cover"
import { useConfirm } from "@/components/confirm"
import { Dropzone } from "@/components/dropzone"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

/** Several readers asking for the same book are one piece of work: grouped by catalogue entry, else by title and author. */
interface RequestGroup { key: string; title: string; author: string; coverUrl: string; requests: BookRequest[] }

function groupRequests(requests: BookRequest[]): RequestGroup[] {
  const groups = new Map<string, RequestGroup>()
  for (const request of requests) {
    const key = request.sourceProvider && request.sourceId ? `${request.sourceProvider}:${request.sourceId}` : `${request.title.trim().toLowerCase()}|${request.author.trim().toLowerCase()}`
    const group = groups.get(key) ?? { key, title: request.title, author: request.author, coverUrl: request.coverUrl, requests: [] }
    group.requests.push(request)
    if (!group.coverUrl && request.coverUrl) group.coverUrl = request.coverUrl
    groups.set(key, group)
  }
  // Most-wanted first, then oldest.
  return [...groups.values()].sort((a, b) => b.requests.length - a.requests.length || a.requests[0].createdAt.localeCompare(b.requests[0].createdAt))
}

const requesters = (requests: BookRequest[]) => requests.map((r) => r.requestedByName || r.requestedByEmail || "A reader")

export function RequestsView() {
  const open = useBookRequests("open")
  const groups = useMemo(() => groupRequests(open.data ?? []), [open.data])
  const [tab, setTab] = useState("open")
  const [fulfilling, setFulfilling] = useState<RequestGroup | null>(null)

  return (
    <>
      <PageHeading title="Requests." description="Books readers have asked for. Add the book and it's marked ready for everyone who asked." />
      <Tabs value={tab} onValueChange={(v) => setTab(String(v))} className="gap-6">
        <TabsList className="w-fit">
          <TabsTrigger value="open"><InboxIcon />Open{groups.length > 0 && <Badge variant="secondary" className="ml-1 tabular-nums">{groups.length}</Badge>}</TabsTrigger>
          <TabsTrigger value="history"><HistoryIcon />History</TabsTrigger>
        </TabsList>
        <TabsContent value="open">
          {open.isPending ? <ListSkeleton /> : open.isError ? (
            <EmptyState icon={<BookMarkedIcon />} title="Requests could not be loaded">{errorMessage(open.error, "Something went wrong.")}</EmptyState>
          ) : groups.length === 0 ? (
            <EmptyState icon={<CheckCircle2Icon />} title="All caught up">When a reader asks for a book that isn't in the library, it shows up here.</EmptyState>
          ) : (
            <ol className="grid divide-y overflow-hidden rounded-xl border bg-background">
              {groups.map((group) => <OpenRow key={group.key} group={group} onFulfill={() => setFulfilling(group)} />)}
            </ol>
          )}
        </TabsContent>
        <TabsContent value="history"><HistoryList /></TabsContent>
      </Tabs>
      <FulfillSheet group={fulfilling} onClose={() => setFulfilling(null)} />
    </>
  )
}

function ListSkeleton() {
  return <div className="grid gap-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-24 rounded-xl" />)}</div>
}

function OpenRow({ group, onFulfill }: { group: RequestGroup; onFulfill: () => void }) {
  const decline = useDeclineBookRequest()
  const confirm = useConfirm()
  const names = requesters(group.requests)
  const declineAll = async () => {
    const ok = await confirm({
      title: `Decline "${group.title}"?`, action: names.length > 1 ? `Decline for ${names.length} readers` : "Decline", destructive: true,
      description: `${names.join(", ")} will see this request was declined. They can ask again later.`,
    })
    if (!ok) return
    try {
      for (const request of group.requests) await decline.mutateAsync(request.id)
      toast.success(`Declined "${group.title}".`)
    } catch (e) { toast.error(errorMessage(e, "Could not decline the request.")) }
  }
  return (
    <li className="flex flex-wrap items-center gap-4 px-4 py-4 animate-in fade-in duration-300 sm:flex-nowrap">
      <Cover book={{ title: group.title, coverUrl: group.coverUrl }} className="w-12 shrink-0 shadow-sm" />
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold text-navy">{group.title}</p>
        {group.author && <p className="truncate text-sm text-muted-foreground">{group.author}</p>}
        <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
          <UsersIcon className="size-3.5" />
          <span className="truncate">{names.join(", ")}</span>
          <span aria-hidden>·</span>
          <span className="shrink-0 tabular-nums">{timeAgo(group.requests[0].createdAt)}</span>
        </p>
      </div>
      {names.length > 1 && <Badge variant="secondary" className="tabular-nums">{names.length} readers</Badge>}
      <div className="flex gap-2">
        <Button variant="ghost" size="sm" disabled={decline.isPending} onClick={declineAll}><XIcon data-icon="inline-start" />Decline</Button>
        <Button size="sm" onClick={onFulfill}><UploadIcon data-icon="inline-start" />Add book</Button>
      </div>
    </li>
  )
}

function HistoryList() {
  const history = useBookRequests("resolved")
  const { books } = useBooks()
  const titles = useMemo(() => new Map(books.map((b) => [b.id, b.title])), [books])
  if (history.isPending) return <ListSkeleton />
  if (history.isError) return <EmptyState icon={<HistoryIcon />} title="History could not be loaded">{errorMessage(history.error, "Something went wrong.")}</EmptyState>
  if (history.data.length === 0) return <EmptyState icon={<HistoryIcon />} title="No history yet">Fulfilled and declined requests will be listed here.</EmptyState>
  return (
    <ol className="grid divide-y overflow-hidden rounded-xl border bg-background">
      {history.data.map((request) => {
        const fulfilled = request.status === "fulfilled"
        const linked = request.fulfilledBookId ? titles.get(request.fulfilledBookId) : undefined
        return (
          <li key={request.id} className="flex items-center gap-4 px-4 py-3.5">
            <Cover book={{ title: request.title, coverUrl: request.coverUrl }} className="w-10 shrink-0 opacity-90" />
            <div className="min-w-0 flex-1">
              <p className="truncate font-semibold text-navy">{request.title}</p>
              <p className="truncate text-sm text-muted-foreground">
                {requesters([request])[0]}
                {fulfilled && linked && linked !== request.title ? ` · added as "${linked}"` : ""}
              </p>
            </div>
            <Badge variant={fulfilled ? "secondary" : "destructive"}>{fulfilled ? "Fulfilled" : "Declined"}</Badge>
            <span className="hidden w-20 text-right text-xs text-muted-foreground tabular-nums sm:block">{request.resolvedAt ? timeAgo(request.resolvedAt) : ""}</span>
          </li>
        )
      })}
    </ol>
  )
}

/**
 * Everything needed to satisfy a request in one place: upload the file (which fulfills every
 * grouped request with the new book), or pick a book that's already in the library. There is
 * no way to mark a request done without real content behind it.
 */
function FulfillSheet({ group, onClose }: { group: RequestGroup | null; onClose: () => void }) {
  // Keep showing the last group while the sheet animates closed.
  const last = useRef<RequestGroup | null>(null)
  if (group) last.current = group
  const shown = group ?? last.current
  return (
    <Sheet open={group !== null} onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent className="data-[side=right]:w-full gap-0 p-0 sm:max-w-lg">
        {shown && <FulfillBody key={shown.key} group={shown} onDone={onClose} />}
      </SheetContent>
    </Sheet>
  )
}

function FulfillBody({ group, onDone }: { group: RequestGroup; onDone: () => void }) {
  const { books, loading } = useBooks()
  const fulfill = useFulfillBookRequest()
  const importBook = useImportBook()
  const updateBook = useUpdateBook()
  const [query, setQuery] = useState(group.title)
  const [progress, setProgress] = useState(0)
  const [duplicate, setDuplicate] = useState<{ bookId: string; message: string } | null>(null)
  const names = requesters(group.requests)

  const matches = useMemo(() => {
    const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
    if (words.length === 0) return []
    return books.filter((b) => words.every((w) => b.title.toLowerCase().includes(w) || b.authors.some((a) => a.toLowerCase().includes(w)))).slice(0, 6)
  }, [books, query])

  const fulfillWith = async (bookId: string, title: string) => {
    try {
      for (const request of group.requests) await fulfill.mutateAsync({ id: request.id, bookId })
      toast.success(names.length > 1 ? `"${title}" is ready for ${names.length} readers.` : `"${title}" is ready for ${names[0]}.`)
      onDone()
    } catch (e) { toast.error(errorMessage(e, "The request could not be fulfilled.")) }
  }

  const upload = async (file: File) => {
    setDuplicate(null)
    setProgress(0)
    try {
      let book: Book = await importBook.mutateAsync({ file, title: "", onProgress: (f) => setProgress(Math.round(f * 100)) })
      // The request often knows a cover the file doesn't have.
      if (!book.coverUrl && /^https?:\/\//.test(group.coverUrl)) {
        book = await updateBook.mutateAsync({ id: book.id, update: { coverUrl: group.coverUrl } }).catch(() => book)
      }
      await fulfillWith(book.id, book.title)
    } catch (e) {
      if (e instanceof APIError && e.code === "duplicate_book" && e.details?.bookId) setDuplicate({ bookId: e.details.bookId, message: e.message })
      else toast.error(errorMessage(e, "The book could not be uploaded."))
    }
  }

  const busy = importBook.isPending || fulfill.isPending
  return (
    <>
      <SheetHeader className="border-b px-6 pt-6 pb-5">
        <div className="flex gap-4 pr-8">
          <Cover book={{ title: group.title, coverUrl: group.coverUrl }} className="w-16 shrink-0 shadow-md" />
          <div className="min-w-0">
            <SheetTitle className="line-clamp-2 text-2xl font-bold text-navy">{group.title}</SheetTitle>
            {group.author && <p className="truncate text-sm text-muted-foreground">{group.author}</p>}
            <SheetDescription className="mt-1.5 flex items-center gap-1.5 text-xs"><UsersIcon className="size-3.5" />Requested by {names.join(", ")}</SheetDescription>
          </div>
        </div>
      </SheetHeader>
      <div className="grid min-h-0 flex-1 content-start gap-8 overflow-y-auto px-6 py-6">
        <ShelfmarkSection group={group} onDone={onDone} />
        <section className="grid gap-3">
          <div>
            <h3 className="text-base font-bold text-navy">Upload the book</h3>
            <p className="text-sm text-muted-foreground">It's added to the library and the request is fulfilled in one step.</p>
          </div>
          <Dropzone accept=".epub,.pdf,application/epub+zip,application/pdf" icon={importBook.isPending ? <Loader2Icon className="animate-spin" /> : <UploadIcon />}
            title={importBook.isPending ? "Uploading…" : "Drop the EPUB or PDF here"} hint="or click to browse" disabled={busy} onFile={upload} />
          {importBook.isPending && (
            <Progress value={progress} aria-label="Upload progress">
              <span className="w-full text-xs text-muted-foreground" role="status">{progress < 100 ? `Uploading… ${progress}%` : "Checking the file…"}</span>
            </Progress>
          )}
          {duplicate && (
            <div className="flex items-center gap-3 rounded-lg bg-mist px-3 py-2.5 text-sm animate-in fade-in duration-200">
              <CopyIcon className="size-4 shrink-0 text-teal-dark" />
              <span className="min-w-0 flex-1">{duplicate.message}.</span>
              <Button size="sm" variant="outline" disabled={busy} onClick={() => fulfillWith(duplicate.bookId, books.find((b) => b.id === duplicate.bookId)?.title ?? group.title)}>Use that book</Button>
            </div>
          )}
        </section>

        <section className="grid gap-3">
          <div>
            <h3 className="text-base font-bold text-navy">Or choose one already in the library</h3>
            <p className="text-sm text-muted-foreground">If it was uploaded another way, link it here.</p>
          </div>
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <label htmlFor="fulfill-search" className="sr-only">Search the library by title or author</label>
            <Input id="fulfill-search" className="pl-9" placeholder="Search by title or author" value={query} onChange={(e) => setQuery(e.target.value)} disabled={loading} />
          </div>
          <ul className="grid gap-1.5" aria-live="polite">
            {query.trim() && matches.length === 0 && !loading && <li className="px-1 py-2 text-sm text-muted-foreground">Nothing in the library matches yet.</li>}
            {matches.map((book) => (
              <li key={book.id}>
                <button type="button" disabled={busy} onClick={() => fulfillWith(book.id, book.title)}
                  className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left transition-colors hover:bg-mist focus-visible:bg-mist focus-visible:outline-none disabled:opacity-60">
                  <Cover book={book} className="w-9 shrink-0" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-navy">{book.title}</span>
                    {book.authors.length > 0 && <span className="block truncate text-xs text-muted-foreground">{book.authors.join(", ")}</span>}
                  </span>
                  <span className="text-xs font-semibold text-teal-dark">Use</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </>
  )
}

/**
 * Finds the book on Shelfmark and downloads it. The server imports the file and fulfills every
 * grouped request when Shelfmark finishes, so the sheet can be closed while it runs.
 * Hidden until Shelfmark is set up in Settings.
 */
function ShelfmarkSection({ group, onDone }: { group: RequestGroup; onDone: () => void }) {
  const client = useQueryClient()
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings })
  const [title, setTitle] = useState(group.title)
  const [queuedId, setQueuedId] = useState<string | null>(null)
  const search = useMutation({ mutationFn: () => api.shelfmarkSearch(title.trim(), group.author) })
  const queue = useMutation({
    mutationFn: (release: unknown) => api.shelfmarkDownload(release, group.requests.map((r) => r.id)),
    onSuccess: (download) => setQueuedId(download.id),
    onError: (e) => toast.error(errorMessage(e, "Shelfmark could not start the download.")),
  })
  const downloads = useQuery({
    queryKey: ["shelfmarkDownloads"],
    queryFn: api.shelfmarkDownloads,
    enabled: queuedId !== null,
    refetchInterval: 2000,
  })
  const download = downloads.data?.items.find((d) => d.id === queuedId)
  const imported = download?.status === "imported"
  // Once only: onDone is a new function on every render of the parent.
  const announced = useRef(false)
  useEffect(() => {
    if (!imported || announced.current) return
    announced.current = true
    void client.invalidateQueries({ queryKey: keys.books })
    void client.invalidateQueries({ queryKey: keys.bookRequests })
    toast.success(`"${group.title}" was downloaded and added.`)
    onDone()
  }, [imported, client, group.title, onDone])

  if (!settings.data?.["shelfmark.url"]?.set) return null
  const submit = (event: FormEvent) => { event.preventDefault(); if (title.trim()) search.mutate() }
  return (
    <section className="grid gap-3">
      <div>
        <h3 className="text-base font-bold text-navy">Get it from Shelfmark</h3>
        <p className="text-sm text-muted-foreground">Download a copy into the library; the request is fulfilled when it arrives.</p>
      </div>
      {download ? (
        <div role="status" className="grid gap-2 rounded-lg bg-mist p-3 text-sm">
          <p className="flex items-center gap-2 font-medium">
            {download.status !== "failed" && <Loader2Icon className="size-4 animate-spin" />}
            {download.status === "failed" ? "The download failed" : download.status === "importing" ? "Adding to the library…" : `Shelfmark: ${download.status}`}
          </p>
          {download.status === "downloading" && <Progress value={download.progress} aria-label="Download progress" />}
          {download.message && <p className={download.status === "failed" ? "text-destructive" : "text-muted-foreground"}>{download.message}</p>}
          {download.status === "failed"
            ? <Button size="sm" variant="outline" className="w-fit" onClick={() => setQueuedId(null)}>Pick another file</Button>
            : <p className="text-xs text-muted-foreground">You can close this; the server finishes it.</p>}
        </div>
      ) : (
        <>
          <form onSubmit={submit} className="flex gap-2">
            <label htmlFor="shelfmark-search" className="sr-only">Title to search Shelfmark for</label>
            <Input id="shelfmark-search" value={title} onChange={(e) => setTitle(e.target.value)} disabled={search.isPending} />
            <Button type="submit" variant="outline" disabled={search.isPending || !title.trim()}>
              {search.isPending ? <Loader2Icon className="animate-spin" /> : <BookDownIcon />}Search
            </Button>
          </form>
          {search.isPending && <p role="status" className="text-sm text-muted-foreground">Searching Shelfmark's sources; this can take a minute…</p>}
          {search.isError && <p role="alert" className="text-sm text-destructive">{errorMessage(search.error, "Shelfmark search failed.")}</p>}
          {search.data && (
            <ul className="grid gap-1.5" aria-live="polite">
              {search.data.items.length === 0 && <li className="px-1 py-2 text-sm text-muted-foreground">Shelfmark found nothing. Try a shorter title.</li>}
              {search.data.items.map((release) => (
                <li key={`${release.source}:${release.sourceId}`} className="flex items-center gap-3 rounded-lg px-2 py-2 hover:bg-mist">
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-navy">{release.title}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {[release.format?.toUpperCase(), release.size, release.language, release.indexer || release.source].filter(Boolean).join(" · ")}
                    </span>
                  </span>
                  <Button size="sm" variant="outline" disabled={queue.isPending} onClick={() => queue.mutate(release.raw)}>
                    <DownloadIcon data-icon="inline-start" />Get
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  )
}
