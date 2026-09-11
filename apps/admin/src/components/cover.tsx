import { ImageIcon } from "lucide-react"
import { useCoverSrc } from "@/lib/queries"
import { initials } from "@/lib/format"
import type { Book } from "@/lib/api"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

export function Cover({ book, className }: { book: Pick<Book, "title" | "coverUrl">; className?: string }) {
  const src = useCoverSrc(book.coverUrl)
  const loading = book.coverUrl.startsWith("/") && !src
  return (
    <div className={cn("relative aspect-[2/3] overflow-hidden rounded-lg bg-gradient-to-br from-navy to-[#315b72]", className)}>
      {src ? (
        <img src={src} alt="" loading="lazy" className="size-full object-cover" />
      ) : loading ? (
        <Skeleton className="size-full rounded-none" />
      ) : (
        <div className="grid size-full place-items-center px-2 text-center font-heading text-white">
          {book.title ? <span className="text-2xl font-bold">{initials(book.title)}</span> : <ImageIcon />}
        </div>
      )}
    </div>
  )
}
