import { useState, type FormEvent, type ReactNode } from "react"
import { BellIcon, BookDownIcon, CodeIcon, InfoIcon, CloudUploadIcon, HardDriveIcon, Loader2Icon, MailIcon, SendIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, APP_VERSION, type Integration, type Settings } from "@/lib/api"
import { keys, useInstance } from "@/lib/queries"
import { errorMessage } from "@/lib/format"
import { useConfirm } from "@/components/confirm"
import { cn } from "@/lib/utils"
import { PageHeading } from "@/components/page-heading"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"

interface Field {
  key: string
  label: string
  placeholder?: string
  hint?: string
  type?: "text" | "email" | "url" | "checkbox"
  /** Half width on wide screens; pairs are placed next to each other on purpose. */
  half?: boolean
}

interface IntegrationCard {
  integration: Integration
  icon: ReactNode
  title: string
  description: string
  fields: Field[]
  /** Keys that must be set for the integration to work. */
  required: string[]
  test: { label: string; done: string }
}

const INTEGRATIONS: IntegrationCard[] = [
  {
    integration: "email", icon: <MailIcon />, title: "Email",
    description: "Resend delivers invites and password-reset codes.",
    required: ["resend.apiKey", "resend.fromEmail"],
    test: { label: "Send test email", done: "Test email sent to your address." },
    fields: [
      { key: "resend.apiKey", label: "Resend API key", placeholder: "re_…" },
      { key: "resend.fromEmail", label: "From address", placeholder: "books@yourdomain.com", type: "email", half: true },
      { key: "resend.fromName", label: "From name", placeholder: "BookHarbor", half: true },
    ],
  },
  {
    integration: "s3", icon: <HardDriveIcon />, title: "Storage",
    description: "Keep book files on this server's disk, or in an S3-compatible bucket such as R2 or MinIO.",
    required: ["s3.endpoint", "s3.bucket", "s3.accessKeyId", "s3.secretKey"],
    test: { label: "Test bucket", done: "Wrote, read, and deleted a test object." },
    fields: [
      { key: "s3.endpoint", label: "Endpoint", placeholder: "https://s3.eu-west-1.amazonaws.com", type: "url", hint: "For Cloudflare R2: https://<account>.r2.cloudflarestorage.com" },
      { key: "s3.bucket", label: "Bucket", placeholder: "bookharbor", half: true },
      { key: "s3.region", label: "Region", placeholder: "us-east-1 (R2: auto)", half: true },
      { key: "s3.accessKeyId", label: "Access key ID", half: true },
      { key: "s3.secretKey", label: "Secret access key", half: true },
      { key: "s3.prefix", label: "Key prefix", placeholder: "optional, e.g. bookharbor/" },
      { key: "s3.pathStyle", label: "Path-style addressing", hint: "Needed by MinIO and most self-hosted stores.", type: "checkbox" },
      { key: "s3.storeUploads", label: "Store new uploads in S3", hint: "Off keeps new books on this server's disk.", type: "checkbox" },
    ],
  },
  {
    integration: "ntfy", icon: <BellIcon />, title: "Notifications",
    description: "ntfy pushes admin alerts: book requests, password resets, storage moves, Shelfmark downloads, and new web novel chapters.",
    required: ["ntfy.topic"],
    test: { label: "Send test push", done: "Test notification sent." },
    fields: [
      { key: "ntfy.url", label: "Server", placeholder: "https://ntfy.sh", type: "url" },
      { key: "ntfy.topic", label: "Topic", placeholder: "a hard-to-guess name", hint: "Anyone who knows a public ntfy.sh topic can read it.", half: true },
      { key: "ntfy.token", label: "Access token", placeholder: "optional", half: true },
    ],
  },
  {
    integration: "shelfmark", icon: <BookDownIcon />, title: "Shelfmark",
    description: "Find and download books for requests from your Shelfmark instance, straight into the library.",
    required: ["shelfmark.url"],
    test: { label: "Test sign-in", done: "Signed in to Shelfmark." },
    fields: [
      { key: "shelfmark.url", label: "Shelfmark URL", placeholder: "http://shelfmark:8084", type: "url", hint: "Where this server can reach Shelfmark." },
      { key: "shelfmark.username", label: "Username", placeholder: "empty if Shelfmark has no login", half: true },
      { key: "shelfmark.password", label: "Password", half: true },
    ],
  },
]

export function SettingsView() {
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings })
  return (
    <>
      <PageHeading title="Settings." description="Connect email, storage, notifications, and Shelfmark. Values saved here override the server's environment." />
      {settings.isPending ? (
        <div className="grid gap-6">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-64 rounded-xl" />)}</div>
      ) : settings.isError ? (
        <p role="alert" className="text-destructive">{errorMessage(settings.error, "Settings could not be loaded.")}</p>
      ) : (
        <div className="grid max-w-6xl divide-y">
          {INTEGRATIONS.map((item) => (
            <Section key={item.integration} item={item} settings={settings.data}>
              {item.integration === "s3" && <StorageMove configured={item.required.every((key) => settings.data[key]?.set)} />}
            </Section>
          ))}
          <About />
        </div>
      )}
    </>
  )
}

function Section({ item, settings, children }: { item: IntegrationCard; settings: Settings; children?: ReactNode }) {
  const client = useQueryClient()
  const [draft, setDraft] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: api.updateSettings,
    onSuccess: (next) => {
      client.setQueryData(["settings"], next)
      setDraft({})
      toast.success(`${item.title} settings saved.`)
      void client.invalidateQueries({ queryKey: keys.instance })
      void client.invalidateQueries({ queryKey: keys.audit })
    },
  })
  const test = useMutation({
    mutationFn: () => api.testIntegration(item.integration),
    onSuccess: () => toast.success(item.test.done),
    onError: (e) => toast.error(errorMessage(e, "The test failed.")),
  })
  const dirty = Object.keys(draft).length > 0
  const configured = item.required.every((key) => settings[key]?.set)
  const submit = (event: FormEvent) => { event.preventDefault(); if (dirty) save.mutate(draft) }
  const edit = (key: string, value: string) => setDraft((d) => {
    const next = { ...d }
    // Back to the saved value means no change; a secret's saved value is never known, so any typing counts.
    if (!settings[key]?.secret && value === (settings[key]?.value ?? "")) delete next[key]
    else next[key] = value
    return next
  })
  const inputs = item.fields.filter((f) => f.type !== "checkbox")
  const checkboxes = item.fields.filter((f) => f.type === "checkbox")

  return (
    <section aria-labelledby={`settings-${item.integration}`} className="grid gap-5 py-8 first:pt-0 lg:grid-cols-[17rem_minmax(0,1fr)] lg:gap-10">
      <header className="flex items-start gap-3.5 lg:flex-col lg:gap-3">
        <span className="grid size-10 shrink-0 place-items-center rounded-full bg-mist text-teal-dark [&_svg]:size-5">{item.icon}</span>
        <div className="grid gap-1.5">
          <h2 id={`settings-${item.integration}`} className="text-xl font-bold text-navy">{item.title}</h2>
          <p className="text-sm text-muted-foreground">{item.description}</p>
          <p className={cn("flex items-center gap-1.5 text-xs font-medium", configured ? "text-teal-dark" : "text-muted-foreground")}>
            <span aria-hidden className={cn("size-2 rounded-full", configured ? "bg-teal" : "bg-muted-foreground/40")} />
            {configured ? "Set up" : "Not set up"}
          </p>
        </div>
      </header>

      <div className="grid min-w-0 content-start gap-4">
        <form onSubmit={submit} className="overflow-hidden rounded-xl border bg-background">
          <div className="grid gap-x-4 gap-y-5 p-5 sm:grid-cols-2 sm:p-6">
            {inputs.map((field) => (
              <SettingInput key={field.key} field={field} saved={settings[field.key]} draft={draft[field.key]} onChange={(v) => edit(field.key, v)} />
            ))}
            {checkboxes.length > 0 && (
              <fieldset className="grid gap-3 border-t pt-5 sm:col-span-2">
                <legend className="sr-only">{item.title} options</legend>
                {checkboxes.map((field) => (
                  <SettingInput key={field.key} field={field} saved={settings[field.key]} draft={draft[field.key]} onChange={(v) => edit(field.key, v)} />
                ))}
              </fieldset>
            )}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t bg-muted/40 px-5 py-3 sm:px-6">
            <p role={save.isError ? "alert" : "status"} className={cn("min-w-0 text-sm", save.isError ? "text-destructive" : "text-muted-foreground")}>
              {save.isError ? errorMessage(save.error, "Settings could not be saved.") : dirty ? "Unsaved changes." : configured ? "Tests use the saved settings." : "Fill in the required fields, save, then test."}
            </p>
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => test.mutate()} disabled={dirty || !configured || test.isPending}
                title={dirty ? "Save first; the test uses saved settings." : configured ? undefined : "Fill in the required settings first."}>
                {test.isPending ? <Loader2Icon className="animate-spin" /> : <SendIcon />}{item.test.label}
              </Button>
              <Button type="submit" disabled={!dirty || save.isPending}>{save.isPending && <Loader2Icon className="animate-spin" />}Save</Button>
            </div>
          </div>
        </form>
        {children}
      </div>
    </section>
  )
}

function SettingInput({ field, saved, draft, onChange }: { field: Field; saved?: Settings[string]; draft?: string; onChange: (value: string) => void }) {
  const id = `setting-${field.key}`
  const fromEnv = saved?.source === "environment"
  if (field.type === "checkbox") {
    const checked = (draft ?? saved?.value) === "true"
    return (
      <label htmlFor={id} className="flex items-start gap-3 text-sm">
        <input id={id} type="checkbox" className="mt-0.5 size-4 shrink-0 accent-teal" checked={checked} onChange={(e) => onChange(e.target.checked ? "true" : "false")} />
        <span className="grid gap-0.5">
          <span className="font-medium">{field.label}</span>
          {(field.hint || fromEnv) && <span className="text-xs text-muted-foreground">{fromEnv ? "Set by the server's environment. " : ""}{field.hint}</span>}
        </span>
      </label>
    )
  }
  const secret = saved?.secret
  const removing = secret && draft === "" && saved?.set
  return (
    <div className={cn("grid content-start gap-2", !field.half && "sm:col-span-2")}>
      <Label htmlFor={id}>{field.label}</Label>
      <div className="flex gap-2">
        <Input id={id} type={secret ? "password" : field.type ?? "text"} autoComplete="off" spellCheck={false}
          value={draft ?? (secret ? "" : saved?.value ?? "")}
          placeholder={secret && saved?.set ? "•••••••• saved; type to replace" : field.placeholder}
          onChange={(e) => onChange(e.target.value)} />
        {secret && saved?.set && saved.source === "database" && draft === undefined && (
          <Button type="button" variant="outline" onClick={() => onChange("")}>Remove</Button>
        )}
      </div>
      {removing ? <p className="text-xs text-destructive">Will be removed when you save.</p>
        : (field.hint || fromEnv) && <p className="text-xs text-muted-foreground">{fromEnv ? "Set by the server's environment; saving here overrides it. " : ""}{field.hint}</p>}
    </div>
  )
}

function StorageMove({ configured }: { configured: boolean }) {
  const client = useQueryClient()
  const confirm = useConfirm()
  const status = useQuery({
    queryKey: ["storage"],
    queryFn: api.storage,
    refetchInterval: (query) => (query.state.data?.moving ? 1500 : false),
  })
  const move = useMutation({
    mutationFn: api.moveToS3,
    onSuccess: () => void client.invalidateQueries({ queryKey: ["storage"] }),
    onError: (e) => toast.error(errorMessage(e, "The move could not start.")),
  })
  const data = status.data
  if (!data) return null
  const start = async () => {
    if (await confirm({
      title: `Move ${data.disk} ${data.disk === 1 ? "file" : "files"} to S3?`,
      description: "Each book file is uploaded, checked, and then deleted from this server's disk. Readers can keep downloading throughout. It runs in the background; you can leave this page.",
      action: "Move to S3",
    })) move.mutate()
  }
  return (
    <div className="grid gap-3 rounded-xl bg-mist p-4 text-sm sm:px-6">
      <p><strong>{data.disk}</strong> {data.disk === 1 ? "book file" : "book files"} on disk · <strong>{data.s3}</strong> in S3</p>
      {data.moving ? (
        <p role="status" className="flex items-center gap-2"><Loader2Icon className="size-4 animate-spin" />Moving to S3… {data.moved} done, {data.disk} to go.</p>
      ) : data.disk > 0 && (
        <Button type="button" variant="outline" className="w-fit" onClick={start} disabled={!configured || move.isPending} title={configured ? undefined : "Save the S3 settings first."}>
          <CloudUploadIcon />Move {data.disk} {data.disk === 1 ? "file" : "files"} to S3
        </Button>
      )}
      {data.error && !data.moving && <p role="alert" className="text-destructive">The last move stopped: {data.error}. Moving again picks up where it left off.</p>}
      <p className="text-xs text-muted-foreground">Files already on disk stay there until you move them. Covers always stay on disk.</p>
    </div>
  )
}

const SOURCE_URL = "https://github.com/ebrahimHakimuddin/book-harbor"
const SUPPORT_URL = "https://www.buymeacoffee.com/kidfury"

function About() {
  const instance = useInstance()
  const server = instance.data?.version ?? "…"
  const commit = instance.data?.commit && instance.data.commit !== "unknown" ? instance.data.commit.slice(0, 7) : null
  return (
    <section aria-labelledby="settings-about" className="grid gap-5 py-8 lg:grid-cols-[17rem_minmax(0,1fr)] lg:gap-10">
      <header className="flex items-start gap-3.5 lg:flex-col lg:gap-3">
        <span className="grid size-10 shrink-0 place-items-center rounded-full bg-mist text-teal-dark [&_svg]:size-5"><InfoIcon /></span>
        <div className="grid gap-1.5">
          <h2 id="settings-about" className="text-xl font-bold text-navy">About</h2>
          <p className="text-sm text-muted-foreground">Versions, source code, and supporting the project.</p>
        </div>
      </header>
      <div className="grid min-w-0 content-start gap-5 rounded-xl border bg-background p-5 sm:p-6">
        <div className="flex items-center gap-4">
          <img src="/admin/mark.png" alt="" className="size-14" />
          <div>
            <p className="font-heading text-xl font-bold text-navy">BookHarbor</p>
            <p className="text-sm text-muted-foreground">Your library. Your harbor. Every device.</p>
          </div>
        </div>
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-sm">
          <dt className="text-muted-foreground">Server</dt>
          <dd className="font-medium tabular-nums">{server}{commit && <span className="ml-2 font-normal text-muted-foreground">({commit})</span>}</dd>
          <dt className="text-muted-foreground">Admin console</dt>
          <dd className="font-medium tabular-nums">{APP_VERSION}</dd>
        </dl>
        <p className="text-xs text-muted-foreground">
          The Android app must be on the same major and minor version as the server (for example 1.0.x with 1.0.x); the server refuses mismatched apps, and the app says which one to update.
        </p>
        <p className="rounded-lg bg-mist p-3 text-xs text-muted-foreground">
          BookHarbor is self-hosted software. The project does not host, provide, or distribute any books or other content; everything in this library was added by whoever runs this server.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="outline" render={<a href={SOURCE_URL} target="_blank" rel="noreferrer" />}><CodeIcon />Source code</Button>
          <a href={SUPPORT_URL} target="_blank" rel="noreferrer">
            <img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me a Coffee" height="40" width="145" className="h-10 w-auto" />
          </a>
        </div>
      </div>
    </section>
  )
}
