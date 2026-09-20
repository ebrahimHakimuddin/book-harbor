import { useEffect, useState, type FormEvent } from "react"
import { ArchiveIcon, BookOpenIcon, HistoryIcon, LogOutIcon, UsersIcon, type LucideIcon, Loader2Icon } from "lucide-react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, type Session, type User } from "@/lib/api"
import { useBooks, useReaders, useUpdateSelf } from "@/lib/queries"
import { errorMessage, initials, passwordOk } from "@/lib/format"
import { cn } from "@/lib/utils"
import { Wordmark } from "@/components/brand"
import { PasswordHelpers, PasswordInput } from "@/components/password-input"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LibraryView } from "@/views/library"
import { ReadersView } from "@/views/readers"
import { ActivityView } from "@/views/activity"
import { BackupView } from "@/views/backup"

const VIEWS = ["library", "readers", "activity", "backup"] as const
type View = (typeof VIEWS)[number]

const NAV: { view: View; label: string; icon: LucideIcon }[] = [
  { view: "library", label: "Library", icon: BookOpenIcon },
  { view: "readers", label: "Readers", icon: UsersIcon },
  { view: "activity", label: "Activity", icon: HistoryIcon },
  { view: "backup", label: "Backup", icon: ArchiveIcon },
]

// The view lives in the URL hash so a refresh keeps you where you were.
function useView(): [View, (view: View) => void] {
  const read = (): View => (VIEWS.find((v) => location.hash === `#/${v}`) ?? "library")
  const [view, setView] = useState(read)
  useEffect(() => {
    const onHash = () => setView(read())
    addEventListener("hashchange", onHash)
    return () => removeEventListener("hashchange", onHash)
  }, [])
  return [view, (next) => { location.hash = `/${next}` }]
}

export function Shell({ session, instanceName }: { session: Session; instanceName: string }) {
  const client = useQueryClient()
  const [view, setView] = useView()
  const [accountOpen, setAccountOpen] = useState(false)
  const books = useBooks()
  const readers = useReaders()
  const counts: Partial<Record<View, number | undefined>> = {
    library: books.loading ? undefined : books.books.length,
    readers: readers.data?.length,
  }

  useEffect(() => { document.title = `${instanceName} administration` }, [instanceName])

  const signOut = async () => { await api.logout(); client.clear() }

  return (
    <div className="min-h-dvh bg-background">
      <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-lg focus:bg-navy focus:px-4 focus:py-2 focus:text-white">Skip to content</a>
      <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-7 lg:h-[72px]">
        <a href="#/library" className="mr-auto lg:w-[244px]" aria-label="BookHarbor administration home"><Wordmark /></a>
        <div className="hidden text-right leading-tight sm:block">
          <div className="text-sm font-semibold text-navy">{instanceName}</div>
          <div className="text-xs text-muted-foreground">Self-hosted</div>
        </div>
        <button type="button" onClick={() => setAccountOpen(true)} aria-label="My account" className="rounded-full focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/60">
          <Avatar><AvatarFallback className="bg-cyan/25 font-heading font-bold text-navy">{initials(session.user.displayName)}</AvatarFallback></Avatar>
        </button>
        <Button variant="outline" onClick={signOut}><LogOutIcon data-icon="inline-start" />Sign out</Button>
      </header>

      <div className="lg:grid lg:grid-cols-[244px_minmax(0,1fr)]">
        <nav aria-label="Administration" className="sticky top-16 z-20 flex gap-1 overflow-x-auto border-b bg-background px-3 py-2 lg:top-[72px] lg:h-[calc(100dvh-72px)] lg:flex-col lg:gap-1 lg:overflow-visible lg:border-r lg:border-b-0 lg:px-5 lg:py-8">
          <p className="mb-2 hidden px-3 text-[0.7rem] font-extrabold tracking-widest text-muted-foreground uppercase lg:block">Manage</p>
          {NAV.map(({ view: id, label, icon: Icon }) => {
            const active = view === id
            return (
              <button key={id} type="button" onClick={() => setView(id)} aria-current={active ? "page" : undefined}
                className={cn(
                  "group relative flex shrink-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-semibold text-foreground transition-colors outline-none hover:bg-mist focus-visible:ring-3 focus-visible:ring-ring/60",
                  active && "bg-accent text-navy",
                )}>
                <span aria-hidden className={cn("absolute top-2 bottom-2 left-0 hidden w-[3px] origin-center scale-y-0 rounded-full bg-teal transition-transform lg:block", active && "scale-y-100")} />
                <Icon className={cn("size-[18px] text-muted-foreground transition-all group-hover:translate-x-0.5", active && "text-teal")} />
                <span>{label}</span>
                {counts[id] !== undefined && <span className="ml-auto text-xs font-medium text-muted-foreground tabular-nums">{counts[id]}</span>}
              </button>
            )
          })}
          <div className="mt-auto hidden px-3 pb-2 lg:block">
            <svg viewBox="0 0 44 14" className="mb-3 h-3.5 w-11 fill-none" strokeWidth="2.4" strokeLinecap="round" aria-hidden>
              <path d="M2 5c5-5 9-5 14 0s9 5 14 0 9-5 12-2" stroke="#2e7d7a" /><path d="M2 11c5-5 9-5 14 0s9 5 14 0 9-5 12-2" stroke="#7fb3c3" />
            </svg>
            <p className="font-heading text-[0.95rem] leading-snug text-muted-foreground italic">A brighter tomorrow,<br />one book at a time.</p>
          </div>
        </nav>

        <main id="main" tabIndex={-1} className="min-w-0 px-4 py-7 outline-none sm:px-8 lg:px-14 lg:py-14">
          <div key={view} className="mx-auto max-w-[1320px] animate-in fade-in slide-in-from-bottom-1 duration-200">
            {view === "library" && <LibraryView />}
            {view === "readers" && <ReadersView currentUserId={session.user.id} />}
            {view === "activity" && <ActivityView />}
            {view === "backup" && <BackupView />}
          </div>
        </main>
      </div>
      <MyAccountDialog user={session.user} open={accountOpen} onClose={() => setAccountOpen(false)} />
    </div>
  )
}

function MyAccountDialog({ user, open, onClose }: { user: User; open: boolean; onClose: () => void }) {
  const update = useUpdateSelf()
  const [displayName, setDisplayName] = useState(user.displayName)
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [reveal, setReveal] = useState(false)

  // Re-seed from the latest user (and clear the password fields) each time the dialog opens.
  useEffect(() => {
    if (!open) return
    setDisplayName(user.displayName); setCurrentPassword(""); setNewPassword(""); setReveal(false); update.reset()
  }, [open, user.displayName])

  const changingPassword = newPassword.length > 0
  const renaming = displayName.trim() !== user.displayName && displayName.trim().length > 0
  const canSubmit = (renaming || changingPassword) && (!changingPassword || (currentPassword.length > 0 && passwordOk(newPassword)))

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!canSubmit) return
    const body: { displayName?: string; currentPassword?: string; newPassword?: string } = {}
    if (renaming) body.displayName = displayName.trim()
    if (changingPassword) { body.currentPassword = currentPassword; body.newPassword = newPassword }
    update.mutate(body, {
      onSuccess: () => { toast.success("Account updated."); setCurrentPassword(""); setNewPassword("") },
    })
  }

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose() }}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle className="text-2xl font-bold text-navy">My account</DialogTitle>
            <DialogDescription>Update your display name, or change your password.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="my-name">Display name</Label>
            <Input id="my-name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={100} autoComplete="off" required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="my-current-password">Current password</Label>
            <PasswordInput id="my-current-password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} visible={reveal} onVisibleChange={setReveal} autoComplete="current-password" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="my-new-password">New password</Label>
            <PasswordInput id="my-new-password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} visible={reveal} onVisibleChange={setReveal} minLength={12} maxLength={1024} autoComplete="new-password" />
            {changingPassword ? <PasswordHelpers value={newPassword} onGenerate={setNewPassword} onReveal={() => setReveal(true)} /> : <p className="text-xs text-muted-foreground">Leave blank to keep your current password.</p>}
          </div>
          <p role="alert" className="min-h-5 text-sm text-destructive">{update.isError && errorMessage(update.error, "Your account could not be updated.")}</p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>Close</Button>
            <Button type="submit" disabled={update.isPending || !canSubmit}>{update.isPending && <Loader2Icon className="animate-spin" />}Save</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
