import { useState, type FormEvent } from "react"
import { CheckCircle2Icon, FileTextIcon, Loader2Icon, UploadIcon, XIcon } from "lucide-react"
import { toast } from "sonner"
import type { Book } from "@/lib/api"
import { useImportBook } from "@/lib/queries"
import { errorMessage, formatBytes } from "@/lib/format"
import { Dropzone } from "@/components/dropzone"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"

export function ImportPanel({ onDone, onCancel }: { onDone: (book: Book) => void; onCancel: () => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState("")
  const [progress, setProgress] = useState(0)
  const importBook = useImportBook()

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!file) return
    setProgress(0)
    importBook.mutate({ file, title, onProgress: (f) => setProgress(Math.round(f * 100)) }, {
      onSuccess: (book) => { toast.success(`${book.title} was imported.`); onDone(book) },
    })
  }

  return (
    <form onSubmit={submit} className="mb-8 grid gap-5 rounded-xl bg-mist p-6 animate-in fade-in slide-in-from-top-2 duration-200">
      <div>
        <h2 className="text-2xl font-bold text-navy">Import a book</h2>
        <p className="mt-1 text-sm text-muted-foreground">Original EPUB and PDF files stay on this server.</p>
      </div>

      {file ? (
        <div className="flex items-center gap-3 rounded-lg border bg-background px-4 py-3 animate-in fade-in zoom-in-95 duration-150">
          <FileTextIcon className="size-5 text-teal" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium text-navy">{file.name}</span>
          <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(file.size)}</span>
          <Button type="button" variant="ghost" size="icon-sm" aria-label="Remove selected file" disabled={importBook.isPending} onClick={() => setFile(null)}><XIcon /></Button>
        </div>
      ) : (
        <Dropzone accept=".epub,.pdf,application/epub+zip,application/pdf" icon={<UploadIcon />} title="Drop an EPUB or PDF here" hint="or click to browse your files" onFile={setFile} />
      )}

      <div className="grid gap-2 sm:max-w-md">
        <Label htmlFor="import-title">Title <span className="font-normal text-muted-foreground">Optional</span></Label>
        <Input id="import-title" value={title} onChange={(e) => setTitle(e.target.value)} maxLength={300} placeholder="Use the title found in the file" className="bg-background" />
      </div>

      {importBook.isPending && (
        <Progress value={progress} className="max-w-md" aria-label="Upload progress">
          <span className="flex w-full items-center gap-2 text-xs text-muted-foreground" role="status">
            {progress < 100 ? <>Uploading… {progress}%</> : <><Loader2Icon className="size-3.5 animate-spin" />Checking the file…</>}
          </span>
        </Progress>
      )}
      {importBook.isSuccess && <p className="flex items-center gap-1.5 text-sm text-success"><CheckCircle2Icon className="size-4" />Imported</p>}
      <p role="alert" className="min-h-5 text-sm text-destructive">{importBook.isError && errorMessage(importBook.error, "The book could not be imported.")}</p>

      <div className="flex gap-2">
        <Button type="submit" disabled={!file || importBook.isPending}>{importBook.isPending ? <Loader2Icon className="animate-spin" /> : <UploadIcon />}Import</Button>
        <Button type="button" variant="outline" onClick={onCancel} disabled={importBook.isPending}>Cancel</Button>
      </div>
    </form>
  )
}
