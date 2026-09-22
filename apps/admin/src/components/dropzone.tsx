import { useId, useState, type ReactNode } from "react"
import { cn } from "@/lib/utils"

/**
 * A file picker that also accepts drops. The native input stays in the tab order
 * (visually hidden), so it is fully keyboard and screen-reader accessible.
 */
export function Dropzone({ accept, onFile, multiple, disabled, icon, title, hint, compact, className }: {
  accept: string
  /** Called once per accepted file; with [multiple], once for each file chosen or dropped. */
  onFile: (file: File) => void
  multiple?: boolean
  disabled?: boolean
  icon: ReactNode
  title: string
  hint: string
  compact?: boolean
  className?: string
}) {
  const id = useId()
  const [over, setOver] = useState(false)
  const allowed = accept.split(",").map((s) => s.trim().toLowerCase())
  const accepts = (file: File) => allowed.some((a) => (a.startsWith(".") ? file.name.toLowerCase().endsWith(a) : file.type === a))

  return (
    <label
      htmlFor={id}
      onDragOver={(e) => { e.preventDefault(); if (!disabled) setOver(true) }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setOver(false)
        if (disabled) return
        const dropped = Array.from(e.dataTransfer.files).filter(accepts)
        ;(multiple ? dropped : dropped.slice(0, 1)).forEach(onFile)
      }}
      className={cn(
        "group flex min-w-0 cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border-[1.5px] border-dashed border-input bg-background text-center transition-all",
        "hover:border-teal hover:bg-mist has-[:focus-visible]:border-ring has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50",
        compact ? "px-4 py-5" : "px-6 py-10",
        over && "scale-[1.01] border-teal bg-mist",
        disabled && "pointer-events-none opacity-50",
        className,
      )}
    >
      <input
        id={id} type="file" accept={accept} multiple={multiple} disabled={disabled} className="sr-only"
        onChange={(e) => { Array.from(e.target.files ?? []).forEach(onFile); e.target.value = "" }}
      />
      <span className={cn("mb-1 text-muted-foreground transition-transform group-hover:-translate-y-0.5 [&_svg]:size-6", over && "-translate-y-1 text-teal-dark")}>{icon}</span>
      <span className="text-sm font-semibold text-navy">{title}</span>
      <span className="text-xs text-muted-foreground">{hint}</span>
    </label>
  )
}
