import { useEffect, useMemo, useRef, useState } from "react"
import { BookOpenIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useBook, useBooks, useDeleteFilter, useSavedFilters, useSaveFilter } from "@/lib/queries"
import { toast } from "sonner"
import type { Book, BookFilter } from "@/lib/api"
import { cn } from "@/lib/utils"
import { Cover } from "@/components/cover"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { BookEditor } from "@/views/book-editor"
import { ImportPanel } from "@/views/import-panel"

const FORMATS = [{ value: "", label: "All" }, { value: "epub", label: "EPUB" }, { value: "pdf", label: "PDF" }]

export function LibraryView() {
  const [query, setQuery] = useState("")
  const [format, setFormat] = useState("")
  const [tag, setTag] = useState("")
  const [series, setSeries] = useState("")
  const [activeFilterId, setActiveFilterId] = useState("")
  const [saving, setSaving] = useState<{ id?: string } | null>(null)
  const [filterName, setFilterName] = useState("")
  const filter = useMemo<BookFilter>(() => ({ q: query.trim(), format, tag: tag.trim(), series: series.trim() }), [query, format, tag, series])
  const [applied, setApplied] = useState(filter)
  useEffect(() => {
    const timer = setTimeout(() => setApplied(filter), 250)
    return () => clearTimeout(timer)
  }, [filter])
  const { books, total, loading, isError, error, hasNextPage, fetchNextPage, isFetchingNextPage, refetch } = useBooks(applied, false)
  const savedFilters = useSavedFilters()
  const saveFilter = useSaveFilter()
  const deleteFilter = useDeleteFilter()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const search = useRef<HTMLInputElement>(null)

  // "/" jumps to search, like most product UIs.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (event.key === "/" && !/^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName) && !target.isContentEditable) {
        event.preventDefault()
        search.current?.focus()
      }
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [])

  const selected = useBook(selectedId)
  const filtering = query.trim() !== "" || format !== "" || tag.trim() !== "" || series.trim() !== ""
  const searching = loading || applied !== filter
  const activeFilter = savedFilters.data?.find((item) => item.id === activeFilterId)
  const clearFilters = () => { setSaving(null); setQuery(""); setFormat(""); setTag(""); setSeries(""); setActiveFilterId("") }
  const applySaved = (id: string) => {
    setSaving(null)
    setActiveFilterId(id)
    const item = savedFilters.data?.find((item) => item.id === id)
    if (!item) { clearFilters(); return }
    setQuery(item.filter.q ?? ""); setFormat(item.filter.format ?? "")
    setTag(item.filter.tag ?? ""); setSeries(item.filter.series ?? "")
  }
  const submitFilter = async (event: React.FormEvent) => {
    event.preventDefault()
    try {
      const item = await saveFilter.mutateAsync({ name: filterName, filter, id: saving?.id })
      setActiveFilterId(item.id); setSaving(null)
      toast.success("Filter saved")
    } catch (error) { toast.error(error instanceof Error ? error.message : "Could not save filter") }
  }

  return (
    <>
      <PageHeading
        title="Your library."
        description="Browse imported and watched books, and keep their catalog details tidy."
        actions={
          <Button size="lg" className="h-10" aria-expanded={importing} onClick={() => setImporting((v) => !v)}>
            <PlusIcon data-icon="inline-start" />Import book
          </Button>
        }
      />

      {importing && <ImportPanel onCancel={() => setImporting(false)} onDone={(book) => { setImporting(false); if (book) setSelectedId(book.id) }} />}

      <div className="mb-6 flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-sm">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <label htmlFor="book-search" className="sr-only">Search books</label>
          <Input id="book-search" ref={search} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search books and metadata" maxLength={300} autoComplete="off" className="h-10 pr-10 pl-9"
            onKeyDown={(e) => { if (e.key === "Escape") { setQuery(""); e.currentTarget.blur() } }} />
          {query ? (
            <Button variant="ghost" size="icon-sm" className="absolute top-1/2 right-1 -translate-y-1/2" aria-label="Clear search" onClick={() => { setQuery(""); search.current?.focus() }}><XIcon /></Button>
          ) : (
            <kbd aria-hidden className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 rounded border bg-muted px-1.5 font-sans text-[0.7rem] text-muted-foreground">/</kbd>
          )}
        </div>
        <ToggleGroup value={[format]} onValueChange={(v) => setFormat(v[0] ?? "")} variant="outline" aria-label="Filter by format">
          {FORMATS.map((f) => <ToggleGroupItem key={f.value} value={f.value} className="px-3.5">{f.label}</ToggleGroupItem>)}
        </ToggleGroup>
        <p role="status" className="ml-auto text-sm text-muted-foreground tabular-nums">
          {searching ? "Searching library…" : `${total} ${total === 1 ? "book" : "books"}${filtering ? total === 1 ? " matches" : " match" : ""}`}
        </p>
      </div>

      <div className="mb-6 grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <label htmlFor="book-tag" className="text-sm font-medium">Tag</label>
          <Input id="book-tag" value={tag} maxLength={300} onChange={(e) => setTag(e.target.value)} placeholder="Any tag" />
        </div>
        <div className="grid gap-1.5">
          <label htmlFor="book-series" className="text-sm font-medium">Series</label>
          <Input id="book-series" value={series} maxLength={300} onChange={(e) => setSeries(e.target.value)} placeholder="Any series" />
        </div>
      </div>
      <div className="mb-6 flex flex-wrap items-end gap-3">
        <div className="grid w-full gap-1.5 sm:w-64">
          <label htmlFor="saved-filter" className="text-sm font-medium">Your saved filters</label>
          <select id="saved-filter" value={activeFilterId} onChange={(e) => applySaved(e.target.value)} disabled={savedFilters.isPending}
            className="h-10 w-full min-w-0 max-w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-ring">
            <option value="">{savedFilters.isPending ? "Loading filters…" : "Choose a saved filter"}</option>
            {savedFilters.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </div>
        <Button variant="outline" onClick={() => { setSaving({}); setFilterName("") }}>Save new filter</Button>
        {activeFilter && <>
          <Button variant="outline" onClick={() => { setSaving({ id: activeFilter.id }); setFilterName(activeFilter.name) }}>Update saved filter</Button>
          <Button variant="ghost" disabled={deleteFilter.isPending} onClick={async () => {
            try { await deleteFilter.mutateAsync(activeFilter.id); setActiveFilterId(""); toast.success("Filter deleted") }
            catch (error) { toast.error(error instanceof Error ? error.message : "Could not delete filter") }
          }}>Delete filter</Button>
        </>}
        {filtering && <Button variant="ghost" onClick={clearFilters}>Clear filters</Button>}
      </div>
      {savedFilters.isError && <p role="alert" className="mb-4 text-sm text-destructive">Saved filters could not be loaded. <Button variant="link" onClick={() => void savedFilters.refetch()}>Try again</Button></p>}
      {saving && <form onSubmit={submitFilter} className="mb-6 flex flex-wrap items-end gap-3">
        <div className="grid w-full gap-1.5 sm:w-72">
          <label htmlFor="filter-name" className="text-sm font-medium">Filter name</label>
          <Input id="filter-name" value={filterName} onChange={(e) => setFilterName(e.target.value)} maxLength={100} required placeholder="e.g. Science fiction EPUBs" />
        </div>
        <Button type="submit" disabled={saveFilter.isPending || !filterName.trim()}>{saveFilter.isPending ? "Saving…" : "Save filter"}</Button>
        <Button type="button" variant="ghost" onClick={() => setSaving(null)}>Cancel</Button>
      </form>}

      {isError ? (
        <EmptyState icon={<BookOpenIcon />} title="The library could not be loaded" action={<Button variant="outline" onClick={() => void refetch()}>Try again</Button>}>
          {error instanceof Error ? error.message : "Something went wrong."}
        </EmptyState>
      ) : loading && books.length === 0 ? (
        <Grid>{Array.from({ length: 8 }, (_, i) => <div key={i} className="grid gap-2"><Skeleton className="aspect-[2/3] rounded-lg" /><Skeleton className="h-4 w-3/4" /><Skeleton className="h-3 w-1/2" /></div>)}</Grid>
      ) : books.length === 0 && !filtering ? (
        <EmptyState icon={<BookOpenIcon />} title="No books yet" action={<Button onClick={() => setImporting(true)}><PlusIcon data-icon="inline-start" />Import your first book</Button>}>
          Import a book or configure a watched folder in Settings to begin your library.
        </EmptyState>
      ) : books.length === 0 ? (
        <EmptyState icon={<SearchIcon />} title="No matches" action={<Button variant="outline" onClick={clearFilters}>Clear filters</Button>}>
          Try different search text, a tag, a series, or a format.
        </EmptyState>
      ) : (
        <Grid>{books.map((book) => <BookTile key={book.id} book={book} selected={book.id === selectedId} onOpen={() => setSelectedId(book.id)} />)}</Grid>
      )}

      {hasNextPage && <div className="mt-7 flex items-center justify-center gap-3">
        <Button variant="outline" disabled={isFetchingNextPage || searching} onClick={() => void fetchNextPage()}>{isFetchingNextPage ? "Loading…" : "Load more books"}</Button>
        <span className="text-sm text-muted-foreground">{books.length} of {total}</span>
      </div>}
      {selected.isError && <p role="alert" className="mt-4 text-sm text-destructive">Book details could not be loaded. <Button variant="link" onClick={() => void selected.refetch()}>Try again</Button></p>}
      <BookEditor book={selected.data ?? null} onClose={() => setSelectedId(null)} />
    </>
  )
}

const Grid = ({ children }: { children: React.ReactNode }) => (
  <div className="grid grid-cols-2 gap-x-5 gap-y-7 sm:grid-cols-[repeat(auto-fill,minmax(168px,1fr))]" aria-live="polite">{children}</div>
)

function BookTile({ book, selected, onOpen }: { book: Book; selected: boolean; onOpen: () => void }) {
  return (
    <button type="button" onClick={onOpen} aria-label={`Edit ${book.title}`}
      className="group grid content-start gap-2.5 rounded-xl p-1.5 text-left outline-none animate-in fade-in zoom-in-95 duration-300 focus-visible:ring-3 focus-visible:ring-ring/70">
      <Cover book={book} className={cn(
        "w-full shadow-[0_7px_24px_rgb(15_45_70/0.10)] ring-teal/0 transition-all duration-200 group-hover:-translate-y-1 group-hover:shadow-[0_14px_36px_rgb(15_45_70/0.18)] group-active:translate-y-0",
        selected && "ring-3 ring-teal ring-offset-2",
      )} />
      <span className="grid gap-0.5">
        <span className="line-clamp-2 text-sm leading-snug font-semibold text-navy">{book.title}</span>
        <span className="line-clamp-1 text-xs text-muted-foreground">{book.authors.join(", ") || book.subtitle || "Metadata not added"}</span>
      </span>
      <span className="flex gap-1">{book.editions.map((e) => <Badge key={e.id} variant="secondary" className="h-5 rounded-md px-1.5 text-[0.65rem] font-bold tracking-wide text-teal-dark uppercase">{e.format}{e.watched ? " · NAS" : ""}</Badge>)}</span>
    </button>
  )
}
