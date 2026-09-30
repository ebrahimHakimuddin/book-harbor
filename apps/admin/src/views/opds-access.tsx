import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, type OPDSConnection, type User } from "@/lib/api"
import { errorMessage } from "@/lib/format"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export function OPDSAccess({ user, onClose }: { user: User; onClose: () => void }) {
  const client = useQueryClient()
  const queryKey = ["opds-credentials", user.id]
  const credentials = useQuery({ queryKey, queryFn: () => api.opdsCredentials(user.id) })
  const [name, setName] = useState("")
  const [connection, setConnection] = useState<OPDSConnection | null>(null)
  const create = useMutation({
    mutationFn: () => api.createOPDSCredential(user.id, name.trim()),
    onSuccess: (result) => {
      setConnection(result); setName("")
      void client.invalidateQueries({ queryKey })
    },
    onError: (error) => toast.error(errorMessage(error, "Could not create external reader access.")),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeOPDSCredential(user.id, id),
    onSuccess: (_, id) => {
      if (connection?.credential.id === id) setConnection(null)
      toast.success("External reader access revoked.")
      void client.invalidateQueries({ queryKey })
    },
    onError: (error) => toast.error(errorMessage(error, "Could not revoke access.")),
  })
  const catalogUrl = new URL(connection?.catalogUrl ?? credentials.data?.catalogUrl ?? "/opds", location.origin).href
  const copy = async () => {
    if (!connection) return
    try {
      await navigator.clipboard.writeText(`Catalog: ${catalogUrl}\nUsername: ${connection.username}\nPassword: ${connection.password}`)
      toast.success("Connection details copied.")
    } catch { toast.error("Could not copy. Select and copy the fields below.") }
  }

  return <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
    <DialogContent className="max-h-[90dvh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>External readers</DialogTitle>
        <DialogDescription>Connect an OPDS app for {user.displayName}. Each connection can browse and download the libraries this account can access.</DialogDescription>
      </DialogHeader>
      <div className="grid gap-4">
        <div className="grid gap-1"><Label htmlFor="opds-url">Catalog URL</Label><Input id="opds-url" readOnly value={catalogUrl} onFocus={(event) => event.target.select()} /></div>
        {credentials.isPending ? <p role="status" className="text-sm">Loading connections…</p> : credentials.isError ? <p role="alert" className="text-sm text-destructive">Connections could not be loaded. <Button variant="link" onClick={() => void credentials.refetch()}>Try again</Button></p> : (
          <ul className="grid gap-2">
            {credentials.data?.items.map((item) => <li key={item.id} className="flex items-center gap-3 rounded-lg border p-3">
              <div className="min-w-0 flex-1"><p className="break-words text-sm font-medium">{item.name}</p><p className="text-xs text-muted-foreground">Created {new Date(item.createdAt).toLocaleDateString()}</p></div>
              <Button variant="outline" disabled={revoke.isPending} onClick={() => revoke.mutate(item.id)}>Revoke</Button>
            </li>)}
            {credentials.data?.items.length === 0 && <li className="text-sm text-muted-foreground">No external reader connections.</li>}
          </ul>
        )}
        {connection && <div role="status" className="grid gap-3 rounded-lg bg-mist p-4">
          <p className="text-sm font-medium">Save these details in your reader app. This password is shown once.</p>
          <div className="grid gap-1"><Label htmlFor="opds-username">Username</Label><Input id="opds-username" readOnly value={connection.username} onFocus={(event) => event.target.select()} /></div>
          <div className="grid gap-1"><Label htmlFor="opds-password">Password</Label><Input id="opds-password" readOnly value={connection.password} autoComplete="off" onFocus={(event) => event.target.select()} /></div>
          <Button className="w-fit" variant="outline" onClick={() => void copy()}>Copy connection details</Button>
        </div>}
        <form onSubmit={(event) => { event.preventDefault(); create.mutate() }} className="grid gap-2 border-t pt-4">
          <Label htmlFor="opds-name">Connection name</Label>
          <Input id="opds-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="KOReader on my tablet" maxLength={100} required disabled={user.disabled || create.isPending} />
          <Button type="submit" className="w-fit" disabled={user.disabled || !name.trim() || create.isPending}>Create connection</Button>
          {user.disabled && <p className="text-sm text-muted-foreground">Enable this account before creating a connection.</p>}
        </form>
        <Button variant="ghost" className="justify-self-end" onClick={onClose}>Close</Button>
      </div>
    </DialogContent>
  </Dialog>
}
