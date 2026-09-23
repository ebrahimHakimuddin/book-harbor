import { useState, type FormEvent, type ReactNode } from "react"
import { BellIcon, CloudUploadIcon, HardDriveIcon, Loader2Icon, MailIcon, SendIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, type Integration, type Settings } from "@/lib/api"
import { keys } from "@/lib/queries"
import { errorMessage } from "@/lib/format"
import { useConfirm } from "@/components/confirm"
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
}

const EMAIL: Field[] = [
  { key: "resend.apiKey", label: "Resend API key", placeholder: "re_…" },
  { key: "resend.fromEmail", label: "From address", placeholder: "books@yourdomain.com", type: "email", hint: "Must be on a domain verified in Resend." },
  { key: "resend.fromName", label: "From name", placeholder: "BookHarbor" },
]

const S3: Field[] = [
  { key: "s3.endpoint", label: "Endpoint", placeholder: "https://s3.eu-west-1.amazonaws.com", type: "url", hint: "For Cloudflare R2: https://<account>.r2.cloudflarestorage.com" },
  { key: "s3.region", label: "Region", placeholder: "us-east-1", hint: "R2 uses auto." },
  { key: "s3.bucket", label: "Bucket", placeholder: "bookharbor" },
  { key: "s3.accessKeyId", label: "Access key ID" },
  { key: "s3.secretKey", label: "Secret access key" },
  { key: "s3.prefix", label: "Key prefix", placeholder: "optional, e.g. bookharbor/" },
  { key: "s3.pathStyle", label: "Path-style addressing (MinIO and most self-hosted stores)", type: "checkbox" },
  { key: "s3.storeUploads", label: "Store new uploads in S3", type: "checkbox", hint: "Off keeps new books on this server's disk." },
]

const NTFY: Field[] = [
  { key: "ntfy.url", label: "Server", placeholder: "https://ntfy.sh", type: "url" },
  { key: "ntfy.topic", label: "Topic", placeholder: "a hard-to-guess topic name", hint: "Anyone who knows a public ntfy.sh topic can read it." },
  { key: "ntfy.token", label: "Access token", placeholder: "optional, for protected topics" },
]

export function SettingsView() {
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings })
  return (
    <>
      <PageHeading title="Settings." description="Connect email, storage, and notifications. Saved values here override the server's environment." />
      {settings.isPending ? (
        <div className="grid max-w-3xl gap-6">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-72 rounded-xl" />)}</div>
      ) : settings.isError ? (
        <p role="alert" className="text-destructive">{errorMessage(settings.error, "Settings could not be loaded.")}</p>
      ) : (
        <div className="grid max-w-3xl gap-6">
          <Section icon={<MailIcon />} title="Email" description="Resend delivers invites and password-reset codes." integration="email" fields={EMAIL} settings={settings.data} />
          <Section icon={<HardDriveIcon />} title="Storage" description="Keep book files on this server's disk, or in an S3-compatible bucket." integration="s3" fields={S3} settings={settings.data}>
            <StorageMove configured={["s3.endpoint", "s3.bucket", "s3.accessKeyId", "s3.secretKey"].every((key) => settings.data[key]?.set)} />
          </Section>
          <Section icon={<BellIcon />} title="Notifications" description="ntfy pushes admin alerts: new book requests, emailed password resets, and storage moves." integration="ntfy" fields={NTFY} settings={settings.data} />
        </div>
      )}
    </>
  )
}

function Section({ icon, title, description, integration, fields, settings, children }: {
  icon: ReactNode; title: string; description: string; integration: Integration; fields: Field[]; settings: Settings; children?: ReactNode
}) {
  const client = useQueryClient()
  const [draft, setDraft] = useState<Record<string, string>>({})
  const save = useMutation({
    mutationFn: api.updateSettings,
    onSuccess: (next) => {
      client.setQueryData(["settings"], next)
      setDraft({})
      toast.success(`${title} settings saved.`)
      void client.invalidateQueries({ queryKey: keys.instance })
      void client.invalidateQueries({ queryKey: keys.audit })
    },
  })
  const test = useMutation({
    mutationFn: () => api.testIntegration(integration),
    onSuccess: () => toast.success(integration === "email" ? "Test email sent to your address." : integration === "s3" ? "Wrote, read, and deleted a test object." : "Test notification sent."),
    onError: (e) => toast.error(errorMessage(e, "The test failed.")),
  })
  const dirty = Object.keys(draft).length > 0
  const submit = (event: FormEvent) => { event.preventDefault(); if (dirty) save.mutate(draft) }
  const edit = (key: string, value: string) => setDraft((d) => {
    const next = { ...d }
    // Back to the saved value means no change; a secret's saved value is never known, so any typing counts.
    if (!settings[key]?.secret && value === (settings[key]?.value ?? "")) delete next[key]
    else next[key] = value
    return next
  })

  return (
    <form onSubmit={submit} className="grid gap-5 rounded-xl border bg-background p-6 sm:p-7">
      <div className="flex items-start gap-4">
        <span className="grid size-11 shrink-0 place-items-center rounded-full bg-mist text-teal-dark [&_svg]:size-5">{icon}</span>
        <div>
          <h2 className="text-2xl font-bold text-navy">{title}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {fields.map((field) => <SettingInput key={field.key} field={field} saved={settings[field.key]} draft={draft[field.key]} onChange={(v) => edit(field.key, v)} />)}
      </div>
      <p role="alert" className="min-h-5 text-sm text-destructive">{save.isError && errorMessage(save.error, "Settings could not be saved.")}</p>
      <div className="flex flex-wrap gap-3">
        <Button type="submit" disabled={!dirty || save.isPending}>{save.isPending && <Loader2Icon className="animate-spin" />}Save</Button>
        <Button type="button" variant="outline" onClick={() => test.mutate()} disabled={dirty || test.isPending} title={dirty ? "Save first; the test uses saved settings." : undefined}>
          {test.isPending ? <Loader2Icon className="animate-spin" /> : <SendIcon />}Send a test
        </Button>
      </div>
      {children}
    </form>
  )
}

function SettingInput({ field, saved, draft, onChange }: { field: Field; saved?: Settings[string]; draft?: string; onChange: (value: string) => void }) {
  const id = `setting-${field.key}`
  const fromEnv = saved?.source === "environment"
  if (field.type === "checkbox") {
    const checked = (draft ?? saved?.value) === "true"
    return (
      <label htmlFor={id} className="flex items-start gap-3 text-sm sm:col-span-2">
        <input id={id} type="checkbox" className="mt-0.5 size-4 accent-teal" checked={checked} onChange={(e) => onChange(e.target.checked ? "true" : "false")} />
        <span>
          <span className="font-medium">{field.label}</span>
          {(field.hint || fromEnv) && <span className="block text-xs text-muted-foreground">{fromEnv ? "Set by the server's environment. " : ""}{field.hint}</span>}
        </span>
      </label>
    )
  }
  const secret = saved?.secret
  return (
    <div className="grid content-start gap-2">
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
      {secret && draft === "" && saved?.set ? <p className="text-xs text-destructive">Will be removed when you save.</p>
        : (field.hint || fromEnv) && <p className="text-xs text-muted-foreground">{fromEnv ? "Set by the server's environment; saving a value here overrides it. " : ""}{field.hint}</p>}
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
    <div className="grid gap-3 rounded-lg bg-mist p-4 text-sm">
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
