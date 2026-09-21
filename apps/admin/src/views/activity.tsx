import { ArchiveIcon, BookOpenIcon, HistoryIcon, RefreshCwIcon, UsersIcon, type LucideIcon } from "lucide-react"
import { useIsFetching } from "@tanstack/react-query"
import { useAudit, keys } from "@/lib/queries"
import { errorMessage, fullDate, timeAgo } from "@/lib/format"
import { cn } from "@/lib/utils"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

const ACTIONS: Record<string, string> = {
  "user.create": "Reader added", "user.update": "Account changed", "user.update_self": "Account updated", "user.delete": "Account removed",
  "book.import": "Book imported", "book.update": "Book edited", "book.delete": "Book deleted", "export.create": "Export downloaded",
  "friend.request": "Friend request sent", "friend.accept": "Friend request accepted", "friend.decline": "Friend request declined",
  "friend.cancel": "Friend request cancelled", "friend.remove": "Friend removed", "social_settings.update": "Sharing settings updated",
  "book_request.create": "Book requested", "book_request.fulfill": "Book request fulfilled", "book_request.decline": "Book request declined", "book_request.cancel": "Book request cancelled",
}

function iconFor(action: string): LucideIcon {
  if (action.startsWith("user.")) return UsersIcon
  if (action.startsWith("book.")) return BookOpenIcon
  if (action.startsWith("friend.") || action.startsWith("social_settings.")) return UsersIcon
  return ArchiveIcon
}

export function ActivityView() {
  const audit = useAudit()
  const fetching = useIsFetching({ queryKey: keys.audit }) > 0
  return (
    <>
      <PageHeading title="Harbor log." description="A record of who changed accounts and books, and when."
        actions={<Button variant="outline" size="lg" className="h-10" onClick={() => audit.refetch()} disabled={fetching}><RefreshCwIcon className={cn(fetching && "animate-spin")} data-icon="inline-start" />Refresh</Button>} />
      {audit.isPending ? (
        <div className="grid gap-3">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-[62px] rounded-xl" />)}</div>
      ) : audit.isError ? (
        <EmptyState icon={<HistoryIcon />} title="Activity could not be loaded">{errorMessage(audit.error, "Something went wrong.")}</EmptyState>
      ) : audit.data.length === 0 ? (
        <EmptyState icon={<HistoryIcon />} title="Nothing yet">Account and book changes will appear here.</EmptyState>
      ) : (
        <ol className="grid divide-y rounded-xl border bg-background">
          {audit.data.map((entry) => {
            const Icon = iconFor(entry.action)
            const destructive = entry.action.endsWith(".delete")
            return (
              <li key={entry.id} className="flex items-center gap-4 px-4 py-3.5 animate-in fade-in duration-300">
                <span className={cn("grid size-9 shrink-0 place-items-center rounded-full", destructive ? "bg-destructive/10 text-destructive" : "bg-mist text-teal-dark")}><Icon className="size-4" /></span>
                <div className="min-w-0 flex-1">
                  <p className="font-semibold text-navy">{ACTIONS[entry.action] ?? entry.action}</p>
                  <p className="truncate text-sm text-muted-foreground">{entry.summary}</p>
                </div>
                <div className="hidden text-right text-xs text-muted-foreground sm:block">by {entry.actorEmail}</div>
                <Tooltip>
                  <TooltipTrigger render={<time dateTime={entry.createdAt} className="shrink-0 text-xs text-muted-foreground tabular-nums" />}>{timeAgo(entry.createdAt)}</TooltipTrigger>
                  <TooltipContent>{fullDate(entry.createdAt)}</TooltipContent>
                </Tooltip>
              </li>
            )
          })}
        </ol>
      )}
    </>
  )
}
