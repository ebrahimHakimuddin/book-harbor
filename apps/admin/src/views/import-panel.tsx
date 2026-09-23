import { useState, type FormEvent } from "react"
import { AlertCircleIcon, CheckCircle2Icon, CopyIcon, FileTextIcon, Loader2Icon, UploadIcon, XIcon } from "lucide-react"
import { toast } from "sonner"
import { APIError, type Book } from "@/lib/api"
import { useImportBook } from "@/lib/queries"
import { errorMessage, formatBytes } from "@/lib/format"
import { BOOK_FILES, Dropzone } from "@/components/dropzone"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"

type Status = { state: "queued" } | { state: "uploading"; progress: number } | { state: "done"; book: Book } | { state: "duplicate"; message: string } | { state: "failed"; message: string }
interface Item { file: File; status: Status }

/** Imports one or many files, one after another; a duplicate or bad file doesn't stop the rest. */
export function ImportPanel({ onDone, onCancel }: { onDone: (book: Book | null) => void; onCancel: () => void }) {
  const [items, setItems] = useState<Item[]>([])
  const [title, setTitle] = useState("")
  const [running, setRunning] = useState(false)
  const importBook = useImportBook()
  const setStatus = (file: File, status: Status) => setItems((list) => list.map((item) => (item.file === file ? { ...item, status } : item)))
  const add = (file: File) => setItems((list) => (list.some((i) => i.file.name === file.name && i.file.size === file.size) ? list : [...list, { file, status: { state: "queued" } }]))
  const pending = items.filter((i) => i.status.state === "queued")

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setRunning(true)
    const imported: Book[] = []
    let failed = 0
    for (const { file } of pending) {
      setStatus(file, { state: "uploading", progress: 0 })
      try {
        const book = await importBook.mutateAsync({ file, title: items.length === 1 ? title : "", onProgress: (f) => setStatus(file, { state: "uploading", progress: Math.round(f * 100) }) })
        imported.push(book)
        setStatus(file, { state: "done", book })
      } catch (error) {
        failed++
        const message = errorMessage(error, "The book could not be imported.")
        setStatus(file, error instanceof APIError && error.code === "duplicate_book" ? { state: "duplicate", message } : { state: "failed", message })
      }
    }
    setRunning(false)
    if (imported.length > 0) toast.success(imported.length === 1 ? `${imported[0].title} was imported.` : `${imported.length} books were imported.`)
    // A single clean import opens straight into its editor; otherwise stay so the results can be read.
    if (imported.length === 1 && failed === 0 && items.length === 1) onDone(imported[0])
  }

  return (
    <form onSubmit={submit} className="mb-8 grid gap-5 rounded-xl bg-mist p-6 animate-in fade-in slide-in-from-top-2 duration-200">
      <div>
        <h2 className="text-2xl font-bold text-navy">Import books</h2>
        <p className="mt-1 text-sm text-muted-foreground">Files stay on this server; MOBI and AZW3 are converted to EPUB. Title, authors, series, and cover are read from each EPUB.</p>
      </div>

      {items.length > 0 && (
        <ul className="grid gap-2" aria-live="polite">
          {items.map(({ file, status }) => (
            <li key={file.name + file.size} className="grid gap-1 rounded-lg border bg-background px-4 py-3 animate-in fade-in zoom-in-95 duration-150">
              <div className="flex items-center gap-3">
                {status.state === "done" ? <CheckCircle2Icon className="size-5 text-success" /> : status.state === "duplicate" ? <CopyIcon className="size-5 text-sunrise" /> : status.state === "failed" ? <AlertCircleIcon className="size-5 text-destructive" /> : status.state === "uploading" ? <Loader2Icon className="size-5 animate-spin text-teal-dark" /> : <FileTextIcon className="size-5 text-teal-dark" />}
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-navy">{status.state === "done" ? status.book.title : file.name}</span>
                <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(file.size)}</span>
                {status.state === "queued" && <Button type="button" variant="ghost" size="icon-sm" aria-label={`Remove ${file.name}`} disabled={running} onClick={() => setItems((list) => list.filter((i) => i.file !== file))}><XIcon /></Button>}
              </div>
              {status.state === "uploading" && (
                <Progress value={status.progress} aria-label={`Uploading ${file.name}`}>
                  <span className="w-full text-xs text-muted-foreground" role="status">{status.progress < 100 ? `Uploading… ${status.progress}%` : "Checking the file…"}</span>
                </Progress>
              )}
              {(status.state === "duplicate" || status.state === "failed") && <p className={status.state === "failed" ? "text-sm text-destructive" : "text-sm text-muted-foreground"}>{status.message}</p>}
            </li>
          ))}
        </ul>
      )}

      <Dropzone multiple compact={items.length > 0} disabled={running} accept={BOOK_FILES} icon={<UploadIcon />}
        title={items.length > 0 ? "Add more files" : "Drop EPUB, PDF, MOBI, or AZW3 files here"} hint="or click to browse your files" onFile={add} />

      {items.length === 1 && (
        <div className="grid gap-2 sm:max-w-md">
          <Label htmlFor="import-title">Title <span className="font-normal text-muted-foreground">Optional</span></Label>
          <Input id="import-title" value={title} onChange={(e) => setTitle(e.target.value)} maxLength={300} placeholder="Use the title found in the file" className="bg-background" disabled={running} />
        </div>
      )}

      <div className="flex gap-2">
        <Button type="submit" disabled={pending.length === 0 || running}>
          {running ? <Loader2Icon className="animate-spin" /> : <UploadIcon />}{pending.length > 1 ? `Import ${pending.length} books` : "Import"}
        </Button>
        <Button type="button" variant="outline" onClick={() => (items.some((i) => i.status.state === "done") ? onDone(null) : onCancel())} disabled={running}>
          {items.some((i) => i.status.state === "done") ? "Done" : "Cancel"}
        </Button>
      </div>
    </form>
  )
}
