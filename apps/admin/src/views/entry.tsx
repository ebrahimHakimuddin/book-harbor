import { useState, type FormEvent } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Loader2Icon, LogInIcon, ShipWheelIcon } from "lucide-react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { keys } from "@/lib/queries"
import { errorMessage, passwordOk } from "@/lib/format"
import { Wordmark } from "@/components/brand"
import { PasswordHelpers, PasswordInput } from "@/components/password-input"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export function EntryScreen({ setup, loadError, email, onEmail }: {
  setup: boolean
  loadError?: string
  email: string
  onEmail: (email: string) => void
}) {
  const client = useQueryClient()
  const [displayName, setDisplayName] = useState("")
  const [password, setPassword] = useState("")
  const [reveal, setReveal] = useState(false)

  const login = useMutation({ mutationFn: () => api.login(email, password) })
  const bootstrap = useMutation({
    mutationFn: () => api.bootstrap({ displayName, email, password }),
    onSuccess: async () => {
      setPassword("")
      toast.success("Administrator created. Sign in to continue.")
      await client.invalidateQueries({ queryKey: keys.instance })
    },
  })
  const mutation = setup ? bootstrap : login
  const error = mutation.isError ? errorMessage(mutation.error, setup ? "Setup could not be completed." : "Sign in failed.") : loadError

  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate() }

  return (
    <div className="grid min-h-dvh bg-sand lg:grid-cols-[minmax(320px,.72fr)_1fr]">
      <div className="flex flex-col justify-between gap-16 p-8 lg:p-16">
        <Wordmark className="text-2xl [&_img]:h-12" />
        <div className="max-w-lg text-navy">
          <h2 className="mb-4 text-5xl leading-none font-bold tracking-tight text-balance lg:text-6xl">Admin</h2>
          <p className="max-w-[38ch] text-muted-foreground">Manage the people and books on this private BookHarbor server.</p>
        </div>
      </div>

      <main className="flex flex-col justify-center bg-background p-8 shadow-[-18px_0_50px_rgb(15_45_70/0.06)] sm:px-16 lg:px-28" id="main">
        <div className="mx-auto w-full max-w-md">
          <h1 className="text-4xl font-bold text-navy sm:text-5xl">{setup ? "Set up your harbor" : "Welcome aboard"}</h1>
          <p className="mt-2 mb-8 text-muted-foreground">
            {setup ? "Create the first administrator for this server." : "Sign in with an administrator account."}
          </p>
          <form onSubmit={submit} className="grid gap-5">
            {setup && (
              <div className="grid gap-2">
                <Label htmlFor="displayName">Display name</Label>
                <Input id="displayName" value={displayName} onChange={(e) => setDisplayName(e.target.value)} autoComplete="name" maxLength={100} required />
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" value={email} onChange={(e) => onEmail(e.target.value)} autoComplete="username" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <PasswordInput id="password" value={password} onChange={(e) => setPassword(e.target.value)} visible={reveal} onVisibleChange={setReveal}
                autoComplete={setup ? "new-password" : "current-password"} minLength={setup ? 12 : undefined} required />
              {setup && <PasswordHelpers value={password} onGenerate={setPassword} onReveal={() => setReveal(true)} />}
            </div>
            <p role="alert" className="min-h-5 text-sm text-destructive">{error}</p>
            <Button type="submit" size="lg" className="h-11 text-base" disabled={mutation.isPending || (setup && !passwordOk(password))}>
              {mutation.isPending ? <Loader2Icon className="animate-spin" /> : setup ? <ShipWheelIcon /> : <LogInIcon />}
              {setup ? "Create administrator" : "Sign in"}
            </Button>
          </form>
        </div>
      </main>
    </div>
  )
}
