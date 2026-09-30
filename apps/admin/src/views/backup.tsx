import { ArchiveIcon, DownloadIcon, Loader2Icon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { keys } from "@/lib/queries"
import { errorMessage } from "@/lib/format"
import { PageHeading } from "@/components/page-heading"
import { Button } from "@/components/ui/button"

export function BackupView() {
  const client = useQueryClient()
  const estimate = useQuery({ queryKey: ["export-estimate"], queryFn: api.exportEstimate })
  const exportArchive = useMutation({
    mutationFn: api.exportArchive,
    onSuccess: (blob, mode) => {
      const url = URL.createObjectURL(blob)
      Object.assign(document.createElement("a"), { href: url, download: `bookharbor-${mode}-${new Date().toISOString().slice(0, 10)}.zip` }).click()
      setTimeout(() => URL.revokeObjectURL(url), 10_000)
      toast.success("Export downloaded.")
      void client.invalidateQueries({ queryKey: keys.audit })
    },
  })
  return (
    <>
      <PageHeading title="Keep it safe." description="Your books and data belong to you. Take a copy any time." />
      <div className="grid max-w-2xl gap-5 rounded-xl bg-mist p-7">
        <span className="grid size-12 place-items-center rounded-full bg-background text-teal-dark shadow-sm"><ArchiveIcon className="size-6" /></span>
        <div>
          <h2 className="text-2xl font-bold text-navy">Export everything</h2>
          <p className="mt-2 text-sm text-muted-foreground">A portable zip with every original EPUB and PDF, including books in watched folders, plus covers and library data. Watched folders must be online during export.</p>
        </div>
        <p className="text-sm">Estimated book files: {estimate.data ? formatBytes(estimate.data.managedBytes + estimate.data.watchedBytes) : "…"}</p>
        <Button size="lg" className="h-10 w-fit" onClick={() => exportArchive.mutate("full")} disabled={exportArchive.isPending}>
          {exportArchive.isPending ? <Loader2Icon className="animate-spin" /> : <DownloadIcon />}{exportArchive.isPending ? "Preparing…" : "Download full export"}
        </Button>
        <div className="border-t pt-5">
          <h3 className="font-bold text-navy">Keep watched files on the NAS</h3>
          <p className="mt-2 text-sm text-muted-foreground">This smaller archive includes managed books and library data, but stores references to {estimate.data?.watchedFiles ?? "…"} watched files. Restore it with the same folders mounted and configured, then scan. Watched files must still exist on the NAS.</p>
          <p className="mt-2 text-sm">Estimated managed book files: {estimate.data ? formatBytes(estimate.data.managedBytes) : "…"}</p>
          <Button type="button" variant="outline" className="mt-4" onClick={() => exportArchive.mutate("references")} disabled={exportArchive.isPending}><DownloadIcon />Download reference backup</Button>
        </div>
        <p role="alert" className="min-h-5 text-sm text-destructive">{exportArchive.isError && errorMessage(exportArchive.error, "The export could not be created.")}</p>
        <p className="text-sm text-muted-foreground">Sign-in sessions are excluded. Password hashes are included, so protect either archive. To restore, stop the server and run <code className="rounded bg-background px-1 py-0.5">bookharbor restore backup.zip</code>.</p>
      </div>
    </>
  )
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  const units = ["KiB", "MiB", "GiB", "TiB"]
  let amount = value
  let unit = -1
  do { amount /= 1024; unit++ } while (amount >= 1024 && unit < units.length - 1)
  return `${amount.toFixed(1)} ${units[unit]}`
}
