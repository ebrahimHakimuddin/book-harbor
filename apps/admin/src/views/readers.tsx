import { useState, type FormEvent } from "react"
import { KeyRoundIcon, Loader2Icon, PauseCircleIcon, PlayCircleIcon, ShieldCheckIcon, ShieldOffIcon, Trash2Icon, UserPlusIcon, UsersIcon } from "lucide-react"
import { toast } from "sonner"
import type { User } from "@/lib/api"
import { useCreateReader, useDeleteReader, useInstance, useReaders, useUpdateReader } from "@/lib/queries"
import { errorMessage, initials, passwordOk, shortDate } from "@/lib/format"
import { cn } from "@/lib/utils"
import { useConfirm } from "@/components/confirm"
import { IconAction } from "@/components/icon-action"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { PasswordHelpers, PasswordInput } from "@/components/password-input"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

export function ReadersView({ currentUserId }: { currentUserId: string }) {
  const readers = useReaders()
  const [justAdded, setJustAdded] = useState<string | null>(null)
  const [resetting, setResetting] = useState<User | null>(null)

  return (
    <>
      <PageHeading title="Your readers." description="Create private accounts for the people sharing this library." />
      <div className="grid items-start gap-9 lg:grid-cols-[minmax(0,1.35fr)_minmax(320px,.65fr)]">
        <div>
          <p role="status" className="mb-3 text-sm text-muted-foreground">
            {readers.data ? `${readers.data.length} ${readers.data.length === 1 ? "account" : "accounts"}` : readers.isError ? errorMessage(readers.error, "Readers could not be loaded.") : "Loading readers…"}
          </p>
          {readers.isPending ? (
            <div className="grid gap-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-[72px] rounded-xl" />)}</div>
          ) : readers.isError ? (
            <EmptyState icon={<UsersIcon />} title="Readers could not be loaded" action={<Button variant="outline" onClick={() => readers.refetch()}>Try again</Button>}>
              {errorMessage(readers.error, "Something went wrong.")}
            </EmptyState>
          ) : readers.data?.length === 0 ? (
            <EmptyState icon={<UsersIcon />} title="No readers yet">Add the first reader with the form.</EmptyState>
          ) : (
            <ul className="grid divide-y rounded-xl border bg-background">
              {readers.data?.map((user) => <ReaderRow key={user.id} user={user} self={user.id === currentUserId} fresh={user.id === justAdded} onReset={() => setResetting(user)} />)}
            </ul>
          )}
        </div>
        <AddReader onCreated={(user) => { setJustAdded(user.id); setTimeout(() => setJustAdded(null), 2500) }} />
      </div>
      <ResetPassword user={resetting} onClose={() => setResetting(null)} />
    </>
  )
}

function ReaderRow({ user, self, fresh, onReset }: { user: User; self: boolean; fresh: boolean; onReset: () => void }) {
  const update = useUpdateReader()
  const remove = useDeleteReader()
  const confirm = useConfirm()
  const fail = (fallback: string) => (e: unknown) => toast.error(errorMessage(e, fallback))
  const admin = user.role === "admin"

  const toggleRole = async () => {
    const next = admin ? "reader" : "admin"
    if (!(await confirm({
      title: `Make ${user.displayName} ${admin ? "a reader" : "an administrator"}?`, action: "Change role",
      description: admin ? "They will lose access to administration." : "Administrators can manage books, accounts, and settings.",
    }))) return
    update.mutate({ id: user.id, role: next }, { onSuccess: () => toast.success(`${user.displayName} is now ${admin ? "a reader" : "an administrator"}.`), onError: fail("The role could not be changed.") })
  }
  const toggleDisabled = async () => {
    if (!user.disabled && !(await confirm({
      title: `Disable ${user.displayName}?`, action: "Disable", destructive: true,
      description: "They are signed out everywhere and cannot sign in until you enable them again. Reading progress is kept.",
    }))) return
    update.mutate({ id: user.id, disabled: !user.disabled }, { onSuccess: () => toast.success(user.disabled ? `${user.displayName} can sign in again.` : `${user.displayName} is disabled.`), onError: fail("The change could not be saved.") })
  }
  const destroy = async () => {
    if (!(await confirm({
      title: `Remove ${user.displayName}?`, action: "Remove reader", destructive: true,
      description: "This deletes the account and its reading progress. Book files are not affected. This cannot be undone.",
    }))) return
    remove.mutate(user.id, { onSuccess: () => toast.success(`${user.displayName} was removed.`), onError: fail("The reader could not be removed.") })
  }

  return (
    <li className={cn("flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3.5 transition-colors first:rounded-t-xl last:rounded-b-xl",
      user.disabled && "bg-muted/50", fresh && "animate-in fade-in slide-in-from-top-2 bg-teal/10 duration-500")}>
      <Avatar size="lg"><AvatarFallback className={cn("bg-cyan/25 font-heading font-bold text-navy", user.disabled && "opacity-60")}>{initials(user.displayName)}</AvatarFallback></Avatar>
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-2 font-semibold text-navy">
          <span className="truncate">{user.displayName}</span>
          {self && <span className="text-xs font-normal text-muted-foreground">(you)</span>}
        </p>
        <p className="truncate text-sm text-muted-foreground">{user.email}</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className={cn("capitalize", admin && "bg-teal/15 text-teal-dark")}>{user.role}</Badge>
        {user.disabled && <Badge variant="outline">Disabled</Badge>}
        <span className="hidden text-xs text-muted-foreground sm:inline">Joined {shortDate(user.createdAt)}</span>
      </div>
      <div className="flex w-full justify-end gap-0.5 sm:w-auto">
        <IconAction label={`Reset password for ${user.displayName}`} icon={KeyRoundIcon} onClick={onReset} />
        {!self && (
          <>
            <IconAction label={admin ? `Make ${user.displayName} a reader` : `Make ${user.displayName} an administrator`} icon={admin ? ShieldOffIcon : ShieldCheckIcon} onClick={toggleRole} disabled={update.isPending} />
            <IconAction label={user.disabled ? `Enable ${user.displayName}` : `Disable ${user.displayName}`} icon={user.disabled ? PlayCircleIcon : PauseCircleIcon} onClick={toggleDisabled} disabled={update.isPending} />
            <IconAction label={`Remove ${user.displayName}`} icon={Trash2Icon} danger onClick={destroy} disabled={remove.isPending} />
          </>
        )}
      </div>
    </li>
  )
}

function AddReader({ onCreated }: { onCreated: (user: User) => void }) {
  const create = useCreateReader()
  const instance = useInstance()
  const invitesEnabled = instance.data?.invitesEnabled ?? false
  const [mode, setMode] = useState<"password" | "invite">("password")
  const [displayName, setDisplayName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [reveal, setReveal] = useState(false)

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const body = mode === "invite" ? { displayName, email, invite: true } : { displayName, email, password }
    create.mutate(body, {
      onSuccess: (user) => {
        toast.success(mode === "invite" ? `An invite email was sent to ${user.email}.` : `${user.displayName} can now sign in.`)
        setDisplayName(""); setEmail(""); setPassword(""); setReveal(false); onCreated(user)
      },
    })
  }

  return (
    <form onSubmit={submit} className="grid gap-4 rounded-xl bg-mist p-6 lg:sticky lg:top-24">
      <div>
        <h2 className="text-2xl font-bold text-navy">Add a reader</h2>
        <p className="mt-1 text-sm text-muted-foreground">{mode === "invite" ? "They'll get an email with a temporary password." : "They can sign in immediately with this password."}</p>
      </div>
      {invitesEnabled && (
        <ToggleGroup value={[mode]} onValueChange={(v) => v[0] && setMode(v[0] as "password" | "invite")} variant="outline" aria-label="How to add this reader">
          <ToggleGroupItem value="password" className="flex-1">Set a password</ToggleGroupItem>
          <ToggleGroupItem value="invite" className="flex-1">Email invite</ToggleGroupItem>
        </ToggleGroup>
      )}
      <div className="grid gap-2"><Label htmlFor="reader-name">Display name</Label><Input id="reader-name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={100} autoComplete="off" required className="bg-background" /></div>
      <div className="grid gap-2"><Label htmlFor="reader-email">Email</Label><Input id="reader-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" required className="bg-background" /></div>
      {mode === "password" && (
        <div className="grid gap-2">
          <Label htmlFor="reader-password">Temporary password</Label>
          <PasswordInput id="reader-password" value={password} onChange={(e) => setPassword(e.target.value)} visible={reveal} onVisibleChange={setReveal} minLength={12} maxLength={1024} autoComplete="new-password" required className="bg-background" />
          <PasswordHelpers value={password} onGenerate={setPassword} onReveal={() => setReveal(true)} />
        </div>
      )}
      <p role="alert" className="min-h-5 text-sm text-destructive">{create.isError && errorMessage(create.error, "The reader could not be created.")}</p>
      <Button type="submit" size="lg" className="h-10" disabled={create.isPending || (mode === "password" && !passwordOk(password))}>
        {create.isPending ? <Loader2Icon className="animate-spin" /> : <UserPlusIcon />}
        {mode === "invite" ? "Send invite" : "Create reader"}
      </Button>
    </form>
  )
}

function ResetPassword({ user, onClose }: { user: User | null; onClose: () => void }) {
  const update = useUpdateReader()
  const [password, setPassword] = useState("")
  const [reveal, setReveal] = useState(false)
  const close = () => { onClose(); setPassword(""); setReveal(false); update.reset() }

  return (
    <Dialog open={user !== null} onOpenChange={(open) => { if (!open) close() }}>
      <DialogContent>
        <form onSubmit={(e) => { e.preventDefault(); if (user) update.mutate({ id: user.id, password }, { onSuccess: () => { toast.success(`Password reset for ${user.displayName}.`); close() } }) }} className="grid gap-4">
          <DialogHeader>
            <DialogTitle className="text-2xl font-bold text-navy">Reset password</DialogTitle>
            <DialogDescription>Set a new password for {user?.displayName}. They will be signed out on every device.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="reset-password">New temporary password</Label>
            <PasswordInput id="reset-password" value={password} onChange={(e) => setPassword(e.target.value)} visible={reveal} onVisibleChange={setReveal} minLength={12} maxLength={1024} autoComplete="new-password" required />
            <PasswordHelpers value={password} onGenerate={setPassword} onReveal={() => setReveal(true)} />
          </div>
          <p role="alert" className="min-h-5 text-sm text-destructive">{update.isError && errorMessage(update.error, "The password could not be reset.")}</p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close}>Cancel</Button>
            <Button type="submit" disabled={update.isPending || !passwordOk(password)}>{update.isPending && <Loader2Icon className="animate-spin" />}Reset password</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
