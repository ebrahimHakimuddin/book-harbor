import { useEffect, useRef, useState, type FormEvent } from "react"
import { CheckIcon, FileTextIcon, ImageIcon, Loader2Icon, PencilIcon, SearchIcon, SparklesIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"
import type { Book, BookUpdate, Candidate, MetadataField } from "@/lib/api"
import { useAddEdition, useCoverMutations, useDeleteBook, useInstance, useLibraries, useLibraryMutations, useMetadataSearch, useUpdateBook, useRefreshBookMetadata } from "@/lib/queries"
import { errorMessage, formatBytes } from "@/lib/format"
import { Cover } from "@/components/cover"
import { useConfirm } from "@/components/confirm"
import { BOOK_FILES, Dropzone } from "@/components/dropzone"
import { EmptyState } from "@/components/page-heading"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"

interface Form { publisher: string; publishedDate: string; language: string; isbn: string; metadataLocks: MetadataField[]; metadataProvenance: Partial<Record<MetadataField, string>>; title: string; subtitle: string; authors: string; description: string; coverUrl: string; series: string; seriesIndex: string; tags: string; source?: { provider: string; id: string } }

const metadataFieldLabels: [MetadataField, string][] = [["title", "Title"], ["subtitle", "Subtitle"], ["authors", "Authors"], ["description", "Description"], ["series", "Series"], ["seriesIndex", "Series number"], ["tags", "Tags"], ["coverUrl", "Cover"], ["publisher", "Publisher"], ["publishedDate", "Publication date"], ["language", "Language"], ["isbn", "ISBN"]]

const isUploaded = (book: Book) => book.coverUrl.startsWith("/")
const fromBook = (book: Book): Form => ({
  publisher: book.publisher ?? "", publishedDate: book.publishedDate ?? "", language: book.language ?? "", isbn: book.isbn ?? "", metadataLocks: [...(book.metadataLocks ?? [])].sort(), metadataProvenance: {},
  title: book.title, subtitle: book.subtitle, authors: book.authors.join("\n"), description: book.description,
  coverUrl: isUploaded(book) ? "" : book.coverUrl, series: book.series ?? "", seriesIndex: book.seriesIndex ? String(book.seriesIndex) : "",
  tags: (book.tags ?? []).join(", "), source: book.source,
})
const splitAuthors = (value: string) => value.split(/\n|,/).map((v) => v.trim()).filter(Boolean)

export function BookEditor({ book, onClose }: { book: Book | null; onClose: () => void }) {
  // Keep showing the last book while the sheet animates closed.
  const last = useRef<Book | null>(null)
  if (book) last.current = book
  const shown = book ?? last.current
  return (
    <Sheet open={book !== null} onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent className="data-[side=right]:w-full gap-0 p-0 sm:max-w-xl">
        {shown && <EditorBody key={shown.id} book={shown} onClose={onClose} />}
      </SheetContent>
    </Sheet>
  )
}

function EditorBody({ book, onClose }: { book: Book; onClose: () => void }) {
  const instance = useInstance()
  const metadataEnabled = instance.data?.metadataEnabled ?? false
  const [tab, setTab] = useState("details")
  const [form, setForm] = useState(() => fromBook(book))
  const set = (patch: Partial<Form>) => setForm((f) => {
    const metadataProvenance = { ...f.metadataProvenance }
    const metadataLocks = new Set(patch.metadataLocks ?? f.metadataLocks)
    for (const key of Object.keys(patch)) {
      if (metadataFieldLabels.some(([field]) => field === key)) {
        metadataProvenance[key as MetadataField] = "manual"
        if (!patch.metadataLocks) metadataLocks.add(key as MetadataField)
      }
    }
    return { ...f, ...patch, metadataLocks: [...metadataLocks].sort(), metadataProvenance }
  })
  const baseline = fromBook(book)
  const dirty = JSON.stringify(form) !== JSON.stringify(baseline)

  // A cover uploaded or removed in the other tab changes what the URL field means.
  useEffect(() => { setForm((f) => ({ ...f, coverUrl: isUploaded(book) ? "" : book.coverUrl, metadataLocks: [...new Set([...f.metadataLocks, ...(book.metadataLocks?.includes("coverUrl") ? ["coverUrl" as MetadataField] : [])])].sort() })) }, [book.coverUrl])

  return (
    <>
      <SheetHeader className="border-b px-6 pt-6 pb-4">
        <SheetTitle className="line-clamp-2 pr-8 text-2xl font-bold text-navy">{book.title}</SheetTitle>
        <SheetDescription>Changes update the local catalog.</SheetDescription>
      </SheetHeader>
      <Tabs value={tab} onValueChange={(v) => setTab(String(v))} className="min-h-0 flex-1 gap-0">
        <TabsList className="mx-6 mt-4 w-fit">
          <TabsTrigger value="details"><PencilIcon />Details{dirty && <span className="size-1.5 rounded-full bg-sunrise" aria-label="Unsaved changes" />}</TabsTrigger>
          <TabsTrigger value="media"><ImageIcon />Cover &amp; files</TabsTrigger>
          {metadataEnabled && <TabsTrigger value="online"><SparklesIcon />Find online</TabsTrigger>}
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          <TabsContent value="details"><DetailsTab book={book} form={form} set={set} dirty={dirty} onSaved={(saved) => setForm(fromBook(saved))} onDeleted={onClose} /></TabsContent>
          <TabsContent value="media"><MediaTab book={book} /></TabsContent>
          {metadataEnabled && <TabsContent value="online"><OnlineTab book={book} onUse={(c) => {
            const patch: Partial<Form> = {}
            const origins = { ...form.metadataProvenance }
            const suggestions: Partial<Form> = { title: c.title || form.title, subtitle: c.subtitle, authors: c.authors.join("\n"), description: c.description,
              ...(c.publishedDate ? { publishedDate: c.publishedDate.slice(0, 10) } : {}), ...(c.isbn13 ? { isbn: c.isbn13 } : {}),
              ...(isUploaded(book) ? {} : { coverUrl: c.coverUrl }) }
            for (const [key, value] of Object.entries(suggestions)) {
              const field = key as MetadataField
              if (!form.metadataLocks.includes(field)) { Object.assign(patch, { [key]: value }); origins[field] = c.provider }
            }
            if (Object.keys(patch).length === 0) { toast.info("All suggested fields are locked. Unlock them in Details first."); return }
            setForm((current) => ({ ...current, ...patch, source: { provider: c.provider, id: c.id }, metadataProvenance: origins, metadataLocks: [...new Set([...current.metadataLocks, ...Object.keys(patch) as MetadataField[]])].sort() }))
            setTab("details")
            toast.success("Unlocked details added to the form. Save to apply them.")
          }} /></TabsContent>}
        </div>
      </Tabs>
    </>
  )
}

function DetailsTab({ book, form, set, dirty, onSaved, onDeleted }: {
  book: Book; form: Form; set: (patch: Partial<Form>) => void; dirty: boolean; onSaved: (book: Book) => void; onDeleted: () => void
}) {
  const update = useUpdateBook()
  const remove = useDeleteBook()
  const refresh = useRefreshBookMetadata()
  const confirm = useConfirm()
  const uploaded = isUploaded(book)
  const libraries = useLibraries()
  const libraryMutations = useLibraryMutations()

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const body: BookUpdate = { metadataLocks: [...form.metadataLocks] }
    const baseline = fromBook(book)
    const fields: MetadataField[] = ["title", "subtitle", "description", "authors", "series", "seriesIndex", "tags", "publisher", "publishedDate", "language", "isbn", "coverUrl"]
    for (const field of fields) {
      if (field === "coverUrl" && uploaded || form[field] === baseline[field]) continue
      const value = field === "authors" || field === "tags" ? splitAuthors(form[field]) : field === "seriesIndex" ? Number(form.seriesIndex) || 0 : form[field]
      Object.assign(body, { [field]: value })
      body.metadataProvenance = { ...body.metadataProvenance, [field]: form.metadataProvenance[field] ?? "manual" }
    }
    if (JSON.stringify(form.source) !== JSON.stringify(book.source)) body.source = form.source
    update.mutate({ id: book.id, update: body }, { onSuccess: (saved) => { onSaved(saved); toast.success("Details saved.") } })
  }

  const destroy = async () => {
    const ok = await confirm({
      title: `Delete "${book.title}"?`, action: "Delete book", destructive: true,
      description: "The book, its files, and every reader's progress on it are removed from this server. This cannot be undone.",
    })
    if (!ok) return
    remove.mutate(book.id, { onSuccess: () => { toast.success(`${book.title} was deleted.`); onDeleted() }, onError: (e) => toast.error(errorMessage(e, "The book could not be deleted.")) })
  }

  return (
    <div className="grid gap-8">
      <div className="grid gap-2">
        <label htmlFor="book-library" className="text-sm font-medium">Library</label>
        <select id="book-library" value={book.libraryId} disabled={libraryMutations.move.isPending || libraries.isPending} className="h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-ring" onChange={async (e) => {
          const libraryId = e.target.value
          const target = libraries.data?.find((item) => item.id === libraryId)
          if (!await confirm({ title: `Move to ${target?.name ?? "this library"}?`, action: "Move book", description: "Readers with access to the destination library can read this book. Other readers lose server access; downloaded copies stay on their devices." })) return
          libraryMutations.move.mutate({ id: book.id, libraryId }, { onSuccess: () => toast.success("Book moved"), onError: (error) => toast.error(errorMessage(error, "Book could not be moved")) })
        }}>
          {libraries.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
        </select>
      </div>
      <form onSubmit={submit} className="grid gap-4">
        <Field label="Title"><Input value={form.title} onChange={(e) => set({ title: e.target.value })} maxLength={300} required /></Field>
        <Field label="Subtitle"><Input value={form.subtitle} onChange={(e) => set({ subtitle: e.target.value })} maxLength={300} /></Field>
        <Field label="Authors"><Textarea value={form.authors} onChange={(e) => set({ authors: e.target.value })} rows={2} maxLength={2000} placeholder="One author per line" /></Field>
        <div className="grid grid-cols-[1fr_7rem] gap-3">
          <Field label="Series"><Input value={form.series} onChange={(e) => set({ series: e.target.value })} maxLength={300} placeholder="Not part of a series" /></Field>
          <Field label="Number"><Input type="number" inputMode="decimal" min={0} step="any" value={form.seriesIndex} onChange={(e) => set({ seriesIndex: e.target.value })} disabled={!form.series.trim()} placeholder="—" /></Field>
        </div>
        <Field label="Publisher"><Input value={form.publisher} onChange={(e) => set({ publisher: e.target.value })} maxLength={300} /></Field>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Publication date" hint="YYYY, YYYY-MM, or YYYY-MM-DD"><Input value={form.publishedDate} onChange={(e) => set({ publishedDate: e.target.value })} maxLength={10} placeholder="2026-09-30" /></Field>
          <Field label="Language" hint="Language tag, such as en or ja"><Input value={form.language} onChange={(e) => set({ language: e.target.value })} maxLength={63} placeholder="en" /></Field>
        </div>
        <Field label="ISBN" hint="ISBN-10 or ISBN-13; spaces and hyphens are accepted."><Input value={form.isbn} onChange={(e) => set({ isbn: e.target.value })} maxLength={30} /></Field>
        <Field label="Tags" hint="Separate with commas. Readers can filter the library by tag."><Input value={form.tags} onChange={(e) => set({ tags: e.target.value })} maxLength={2000} placeholder="Fantasy, Book club" /></Field>
        <Field label="Description"><Textarea value={form.description} onChange={(e) => set({ description: e.target.value })} rows={6} maxLength={10000} /></Field>
        <Field label="Cover URL" hint={uploaded ? "An uploaded cover is in use. Remove it on the Cover & files tab to use a web address instead." : undefined}>
          <Input type="url" value={form.coverUrl} onChange={(e) => set({ coverUrl: e.target.value })} maxLength={2048} placeholder={uploaded ? "Using uploaded image" : "https://"} disabled={uploaded} />
        </Field>
        <section className="grid gap-3 border-t pt-4" aria-labelledby="metadata-locks-heading">
          <h3 id="metadata-locks-heading" className="text-base font-bold text-navy">Protect metadata</h3>
          <p className="text-sm text-muted-foreground">Locked fields keep their values during EPUB refreshes and online suggestions. You can still edit them here. Editing a field locks it; uncheck it before saving to allow future refreshes.</p>
          <div className="grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-2">
            {metadataFieldLabels.map(([field, label]) => <label key={field} className="flex min-h-11 items-center gap-3 text-sm">
              <input type="checkbox" checked={form.metadataLocks.includes(field)} className="size-4 shrink-0 accent-teal-dark" onChange={(event) => set({ metadataLocks: event.target.checked ? [...form.metadataLocks, field].sort() : form.metadataLocks.filter((key) => key !== field) })} />
              <span>{label}<span className="block text-xs text-muted-foreground">{form.metadataProvenance[field] ?? book.metadataProvenance?.[field] ?? "Not recorded"}</span></span>
            </label>)}
          </div>
          <p className="text-xs text-muted-foreground">Legacy values are protected because their original source was not recorded. Cover images are managed separately and are never replaced by a metadata refresh.</p>
        </section>
        {form.source && <p className="text-xs text-muted-foreground">Matched with {form.source.provider}.</p>}
        <p role="alert" className="min-h-5 text-sm text-destructive">{update.isError && errorMessage(update.error, "Details could not be saved.")}</p>
        <Button type="submit" className="w-fit" disabled={!dirty || update.isPending}>
          {update.isPending ? <Loader2Icon className="animate-spin" /> : <CheckIcon />}{dirty ? "Save details" : "Saved"}
        </Button>
      </form>

      <section className="grid gap-3 border-t pt-6">
        <h3 className="text-base font-bold text-navy">Refresh from EPUB</h3>
        <p className="text-sm text-muted-foreground">Re-read the EPUB’s embedded metadata into unlocked fields. Missing values keep the current details. Save or discard your edits first.</p>
        <Button variant="outline" className="w-fit" disabled={dirty || refresh.isPending || update.isPending || !book.editions.some((e) => e.format === "epub")} onClick={() => refresh.mutate(book.id, { onSuccess: (saved) => { onSaved(saved); toast.success("Unlocked metadata refreshed.") } })}>
          {refresh.isPending ? <Loader2Icon className="animate-spin" /> : <SparklesIcon />}Refresh metadata
        </Button>
        <p role="alert" className="min-h-5 text-sm text-destructive">{refresh.isError && errorMessage(refresh.error, "Metadata could not be refreshed.")}</p>
      </section>
      <section className="border-t pt-6">
        <h3 className="text-base font-bold text-navy">Delete this book</h3>
        <p className="mt-1 mb-4 text-sm text-muted-foreground">Removes the book, its files, and every reader's progress from this server. Copies already downloaded to devices are not affected.</p>
        <Button variant="destructive" onClick={destroy} disabled={remove.isPending}>{remove.isPending ? <Loader2Icon className="animate-spin" /> : <Trash2Icon />}Delete book</Button>
      </section>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <Label className="grid gap-2 text-sm font-semibold text-navy">{label}{children}{hint && <span className="text-xs font-normal text-muted-foreground">{hint}</span>}</Label>
}

function MediaTab({ book }: { book: Book }) {
  const cover = useCoverMutations()
  const addEdition = useAddEdition()
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState<{ cover?: string; edition?: string }>({})
  const uploaded = isUploaded(book)

  return (
    <div className="grid gap-8">
      <section className="min-w-0">
        <h3 className="mb-3 text-base font-bold text-navy">Cover</h3>
        <div className="grid grid-cols-[96px_1fr] gap-4">
          <Cover book={book} className="w-24 shadow-md" />
          <div className="grid min-w-0 content-start gap-3">
            <Dropzone compact accept="image/png,image/jpeg,image/webp" icon={cover.upload.isPending ? <Loader2Icon className="animate-spin" /> : <ImageIcon />} title={cover.upload.isPending ? "Uploading…" : "Drop an image"} hint="PNG, JPEG, or WebP up to 2 MiB" disabled={cover.upload.isPending}
              onFile={(file) => { setError((e) => ({ ...e, cover: undefined })); cover.upload.mutate({ id: book.id, file }, { onSuccess: () => toast.success("Cover updated."), onError: (e) => setError((s) => ({ ...s, cover: errorMessage(e, "The cover could not be uploaded.") })) }) }} />
            {uploaded && <Button variant="outline" size="sm" className="w-fit" disabled={cover.remove.isPending}
              onClick={() => cover.remove.mutate(book.id, { onSuccess: () => toast.success("Cover removed."), onError: (e) => setError((s) => ({ ...s, cover: errorMessage(e, "The cover could not be removed.") })) })}><Trash2Icon />Remove uploaded cover</Button>}
          </div>
        </div>
        <p role="alert" className="mt-2 min-h-5 text-sm text-destructive">{error.cover}</p>
      </section>

      <section className="min-w-0">
        <h3 className="mb-3 text-base font-bold text-navy">Formats</h3>
        <ul className="mb-4 grid gap-2">
          {book.editions.map((edition) => (
            <li key={edition.id} className="flex min-w-0 items-center gap-3 rounded-lg bg-mist px-3 py-2.5">
              <FileTextIcon className="size-4 text-teal-dark" />
              <span className="text-xs font-bold tracking-wide text-teal-dark uppercase">{edition.format}</span>
              <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">{edition.originalFilename}</span>
              <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(edition.byteLength)}</span>
            </li>
          ))}
        </ul>
        <Dropzone compact accept={BOOK_FILES} icon={<FileTextIcon />} title="Add another format" hint="One EPUB (MOBI and AZW3 become EPUB) and one PDF" disabled={addEdition.isPending}
          onFile={(file) => { setError((e) => ({ ...e, edition: undefined })); setProgress(0)
            addEdition.mutate({ id: book.id, file, onProgress: (f) => setProgress(Math.round(f * 100)) }, { onSuccess: () => toast.success(`${file.name} was added.`), onError: (e) => setError((s) => ({ ...s, edition: errorMessage(e, "The file could not be added.") })) }) }} />
        {addEdition.isPending && <Progress value={progress} className="mt-3" aria-label="Upload progress"><span className="w-full text-xs text-muted-foreground" role="status">Uploading… {progress}%</span></Progress>}
        <p role="alert" className="mt-2 min-h-5 text-sm text-destructive">{error.edition}</p>
      </section>
    </div>
  )
}

function OnlineTab({ book, onUse }: { book: Book; onUse: (candidate: Candidate) => void }) {
  const [query, setQuery] = useState(book.title)
  const search = useMetadataSearch()
  const items = search.data?.items
  return (
    <div className="grid gap-4">
      <div>
        <h3 className="text-base font-bold text-navy">Find details on Hardcover</h3>
        <p className="mt-1 text-sm text-muted-foreground">Search results are suggestions. Review them before saving.</p>
      </div>
      <form onSubmit={(e) => { e.preventDefault(); search.mutate(query) }} className="flex gap-2">
        <label htmlFor="hardcover-query" className="sr-only">Title or author</label>
        <Input id="hardcover-query" value={query} onChange={(e) => setQuery(e.target.value)} minLength={2} maxLength={200} placeholder="Title or author" required />
        <Button type="submit" disabled={search.isPending}>{search.isPending ? <Loader2Icon className="animate-spin" /> : <SearchIcon />}Search</Button>
      </form>
      <p role="alert" className="min-h-5 text-sm text-destructive">{search.isError && errorMessage(search.error, "Hardcover search could not be completed.")}</p>
      {items?.length === 0 && <EmptyState icon={<SearchIcon />} title="No matches">Try a shorter title or add the author's name.</EmptyState>}
      <ul className="grid gap-2" aria-live="polite">
        {items?.map((c) => (
          <li key={c.id} className="flex min-w-0 items-center gap-3 rounded-lg bg-mist p-2.5 animate-in fade-in slide-in-from-bottom-1 duration-200">
            {c.coverUrl ? <img src={c.coverUrl} alt="" className="aspect-[2/3] w-11 rounded object-cover" /> : <div className="aspect-[2/3] w-11 rounded bg-border" />}
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-navy">{c.title}</p>
              <p className="truncate text-xs text-muted-foreground">{c.authors.join(", ") || c.publishedDate || "Hardcover catalog"}</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => onUse(c)}>Use</Button>
          </li>
        ))}
      </ul>
    </div>
  )
}
