import { useEffect, useMemo, useRef, useState } from "react"
import { BookOpenIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useBooks } from "@/lib/queries"
import type { Book } from "@/lib/api"
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
  const { books, loading, isError, error } = useBooks()
  const [query, setQuery] = useState("")
  const [format, setFormat] = useState("")
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

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return books.filter((book) =>
      (!format || book.editions.some((e) => e.format === format)) &&
      (!needle || [book.title, book.subtitle, ...book.authors].some((v) => v.toLowerCase().includes(needle))))
  }, [books, query, format])

  const selected = books.find((b) => b.id === selectedId) ?? null
  const filtering = query.trim() !== "" || format !== ""

  return (
    <>
      <PageHeading
        title="Your library."
        description="Import books and keep their catalog details tidy."
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
          <Input id="book-search" ref={search} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search title or author" autoComplete="off" className="h-10 pr-10 pl-9"
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
          {loading ? "Loading library…" : filtering ? `${filtered.length} of ${books.length} books` : `${books.length} ${books.length === 1 ? "book" : "books"}`}
        </p>
      </div>

      {isError ? (
        <EmptyState icon={<BookOpenIcon />} title="The library could not be loaded" action={<Button variant="outline" onClick={() => location.reload()}>Try again</Button>}>
          {error instanceof Error ? error.message : "Something went wrong."}
        </EmptyState>
      ) : loading && books.length === 0 ? (
        <Grid>{Array.from({ length: 8 }, (_, i) => <div key={i} className="grid gap-2"><Skeleton className="aspect-[2/3] rounded-lg" /><Skeleton className="h-4 w-3/4" /><Skeleton className="h-3 w-1/2" /></div>)}</Grid>
      ) : books.length === 0 ? (
        <EmptyState icon={<BookOpenIcon />} title="No books yet" action={<Button onClick={() => setImporting(true)}><PlusIcon data-icon="inline-start" />Import your first book</Button>}>
          Import an EPUB, PDF, MOBI, or AZW3 to begin your library.
        </EmptyState>
      ) : filtered.length === 0 ? (
        <EmptyState icon={<SearchIcon />} title="No matches" action={<Button variant="outline" onClick={() => { setQuery(""); setFormat("") }}>Clear filters</Button>}>
          Try a different title, author, or format.
        </EmptyState>
      ) : (
        <Grid>{filtered.map((book) => <BookTile key={book.id} book={book} selected={book.id === selectedId} onOpen={() => setSelectedId(book.id)} />)}</Grid>
      )}

      <BookEditor book={selected} onClose={() => setSelectedId(null)} />
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
      <span className="flex gap-1">{book.editions.map((e) => <Badge key={e.id} variant="secondary" className="h-5 rounded-md px-1.5 text-[0.65rem] font-bold tracking-wide text-teal-dark uppercase">{e.format}</Badge>)}</span>
    </button>
  )
}
