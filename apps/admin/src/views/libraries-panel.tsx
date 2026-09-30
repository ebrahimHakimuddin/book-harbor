import { useState, type FormEvent } from "react"
import { toast } from "sonner"
import type { CatalogLibraryInput } from "@/lib/api"
import { useLibraries, useLibraryMutations, useReaders } from "@/lib/queries"
import { errorMessage } from "@/lib/format"
import { useConfirm } from "@/components/confirm"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

type Draft = CatalogLibraryInput & { id?: string }

export function LibrariesPanel({ onClose }: { onClose: () => void }) {
  const libraries = useLibraries()
  const readers = useReaders()
  const mutations = useLibraryMutations()
  const confirm = useConfirm()
  const [draft, setDraft] = useState<Draft | null>(null)
  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!draft) return
    try {
      await mutations.save.mutateAsync({ id: draft.id, body: { name: draft.name, allReaders: draft.allReaders, readerIds: draft.readerIds } })
      setDraft(null); toast.success("Library saved")
    } catch (error) { toast.error(errorMessage(error, "Library could not be saved")) }
  }
  return (
    <section aria-labelledby="libraries-heading" className="mb-8 grid gap-5 border-b pb-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="libraries-heading" className="text-2xl font-bold">Libraries and access</h2>
        <div className="flex gap-2">
          <Button variant="outline" disabled={mutations.save.isPending} onClick={() => setDraft({ name: "", allReaders: false, readerIds: [] })}>New library</Button>
          <Button variant="ghost" disabled={mutations.save.isPending} onClick={onClose}>Close libraries</Button>
        </div>
      </div>
      <p className="max-w-prose text-sm text-muted-foreground">Each book belongs to one library. Administrators can access every library. Reader access changes immediately on the server; downloaded copies remain on readers’ devices.</p>
      {libraries.isPending ? <p role="status">Loading libraries…</p> : libraries.isError ? <p role="alert" className="text-destructive">Libraries could not be loaded. <Button variant="link" onClick={() => void libraries.refetch()}>Try again</Button></p> : (
        <ul className="divide-y">
          {libraries.data?.map((library) => <li key={library.id} className="flex flex-wrap items-center gap-3 py-3">
            <div className="min-w-0 flex-1">
              <p className="break-words font-semibold">{library.name}</p>
              <p className="text-sm text-muted-foreground">{library.bookCount} {library.bookCount === 1 ? "book" : "books"} · {library.allReaders ? "All readers" : `${library.readerIds?.length ?? 0} selected accounts`}</p>
            </div>
            <Button variant="outline" disabled={mutations.save.isPending} onClick={() => setDraft({ id: library.id, name: library.name, allReaders: library.allReaders, readerIds: library.readerIds ?? [] })}>Edit {library.name}</Button>
            <Button variant="ghost" disabled={library.id === "library_main" || library.bookCount > 0 || mutations.remove.isPending || mutations.save.isPending} title={library.id === "library_main" ? "The main library is kept for new imports" : library.bookCount > 0 ? "Move the books before deleting this library" : undefined} onClick={async () => {
              if (!await confirm({ title: `Delete ${library.name}?`, action: "Delete library", description: "This empty library and its reader access settings will be removed." })) return
              try { await mutations.remove.mutateAsync(library.id); toast.success("Library deleted") }
              catch (error) { toast.error(errorMessage(error, "Library could not be deleted")) }
            }}>Delete {library.name}</Button>
          </li>)}
        </ul>
      )}
      {draft && <form onSubmit={submit} className="grid max-w-xl gap-4">
        <h3 className="text-lg font-semibold">{draft.id ? "Edit library" : "New library"}</h3>
        <div className="grid gap-1.5">
          <label htmlFor="library-name" className="text-sm font-medium">Library name</label>
          <Input id="library-name" value={draft.name} maxLength={100} required disabled={mutations.save.isPending} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </div>
        <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={draft.allReaders} disabled={mutations.save.isPending} onChange={(e) => setDraft({ ...draft, allReaders: e.target.checked })} className="size-4 accent-teal" />All readers, including new accounts</label>
        <fieldset disabled={draft.allReaders || mutations.save.isPending || readers.isPending || readers.isError} className="grid gap-2">
          <legend className="mb-2 text-sm font-medium">Selected accounts</legend>
          {readers.isPending ? <p role="status">Loading accounts…</p> : readers.data?.filter((user) => user.role !== "admin").map((user) => <label key={user.id} className="flex items-center gap-3 text-sm">
            <input type="checkbox" checked={draft.readerIds.includes(user.id)} onChange={(e) => setDraft({ ...draft, readerIds: e.target.checked ? [...draft.readerIds, user.id] : draft.readerIds.filter((id) => id !== user.id) })} className="size-4 accent-teal" />
            <span className="min-w-0 break-words">{user.displayName} · {user.email}{user.disabled ? " (disabled)" : ""}</span>
          </label>)}
        </fieldset>
        {readers.isError && <p role="alert" className="text-sm text-destructive">Accounts could not be loaded. <Button type="button" variant="link" onClick={() => void readers.refetch()}>Try again</Button></p>}
        {!draft.allReaders && draft.readerIds.length === 0 && <p className="text-sm text-muted-foreground">Only administrators will have access.</p>}
        <div className="flex gap-3">
          <Button type="submit" disabled={!draft.name.trim() || mutations.save.isPending}>{mutations.save.isPending ? "Saving…" : "Save library"}</Button>
          <Button type="button" variant="ghost" disabled={mutations.save.isPending} onClick={() => setDraft(null)}>Cancel</Button>
        </div>
      </form>}
    </section>
  )
}
