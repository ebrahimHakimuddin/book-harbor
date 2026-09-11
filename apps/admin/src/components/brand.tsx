import { cn } from "@/lib/utils"

export function BrandMark({ className }: { className?: string }) {
  return <img src="/admin/mark.png" alt="" className={cn("h-9 w-auto", className)} />
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("flex items-center gap-2.5 font-heading text-xl font-bold text-navy", className)}>
      <BrandMark />
      BookHarbor
    </span>
  )
}
