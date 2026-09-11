import { ArchiveIcon, DownloadIcon, Loader2Icon } from "lucide-react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { keys } from "@/lib/queries"
import { errorMessage } from "@/lib/format"
import { PageHeading } from "@/components/page-heading"
import { Button } from "@/components/ui/button"

export function BackupView() {
  const client = useQueryClient()
  const exportArchive = useMutation({
    mutationFn: api.exportArchive,
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob)
      Object.assign(document.createElement("a"), { href: url, download: `bookharbor-export-${new Date().toISOString().slice(0, 10)}.zip` }).click()
      setTimeout(() => URL.revokeObjectURL(url), 10_000)
      toast.success("Export downloaded.")
      void client.invalidateQueries({ queryKey: keys.audit })
    },
  })
  return (
    <>
      <PageHeading title="Keep it safe." description="Your books and data belong to you. Take a copy any time." />
      <div className="grid max-w-2xl gap-5 rounded-xl bg-mist p-7">
        <span className="grid size-12 place-items-center rounded-full bg-background text-teal shadow-sm"><ArchiveIcon className="size-6" /></span>
        <div>
          <h2 className="text-2xl font-bold text-navy">Export everything</h2>
          <p className="mt-2 text-sm text-muted-foreground">One zip with every original EPUB and PDF, uploaded covers, a manifest, and a snapshot of the library database. Sign-in sessions are left out; password hashes are included, so store the file as carefully as the server.</p>
        </div>
        <Button size="lg" className="h-10 w-fit" onClick={() => exportArchive.mutate()} disabled={exportArchive.isPending}>
          {exportArchive.isPending ? <Loader2Icon className="animate-spin" /> : <DownloadIcon />}{exportArchive.isPending ? "Preparing…" : "Download export"}
        </Button>
        <p role="alert" className="min-h-5 text-sm text-destructive">{exportArchive.isError && errorMessage(exportArchive.error, "The export could not be created.")}</p>
      </div>
    </>
  )
}
