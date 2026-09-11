import { useState, type ComponentProps } from "react"
import { CheckIcon, ClipboardCopyIcon, EyeIcon, EyeOffIcon, SparklesIcon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { generatePassword, passwordOk } from "@/lib/format"
import { cn } from "@/lib/utils"

export function PasswordInput({ className, visible, onVisibleChange, ...props }: ComponentProps<"input"> & {
  visible?: boolean
  onVisibleChange?: (visible: boolean) => void
}) {
  const [own, setOwn] = useState(false)
  const shown = visible ?? own
  return (
    <div className="relative">
      <Input {...props} type={shown ? "text" : "password"} className={cn("pr-10", className)} />
      <Button
        type="button" variant="ghost" size="icon-sm"
        className="absolute top-1/2 right-1 -translate-y-1/2 text-muted-foreground"
        aria-label={shown ? "Hide password" : "Show password"} aria-pressed={shown}
        onClick={() => (onVisibleChange ?? setOwn)(!shown)}
      >
        {shown ? <EyeOffIcon /> : <EyeIcon />}
      </Button>
    </div>
  )
}

/** Generate / copy helpers and a live length hint for a controlled password field. */
export function PasswordHelpers({ value, onGenerate, onReveal }: {
  value: string
  onGenerate: (password: string) => void
  onReveal: () => void
}) {
  const ok = passwordOk(value)
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
      <Button type="button" variant="ghost" size="sm" onClick={() => { onGenerate(generatePassword()); onReveal() }}>
        <SparklesIcon data-icon="inline-start" />Generate
      </Button>
      <Button type="button" variant="ghost" size="sm" disabled={!value}
        onClick={() => navigator.clipboard.writeText(value).then(() => toast.success("Password copied"), () => toast.error("Copy is unavailable here"))}>
        <ClipboardCopyIcon data-icon="inline-start" />Copy
      </Button>
      <span className={cn("flex items-center gap-1 text-xs transition-colors", ok ? "text-success" : "text-muted-foreground")} aria-live="polite">
        {ok && <CheckIcon className="size-3.5" />}
        {ok ? "Long enough" : `${value.length}/12 characters minimum`}
      </span>
    </div>
  )
}
