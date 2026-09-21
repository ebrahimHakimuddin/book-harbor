import { useMemo, useState } from "react"
import { BookMarkedIcon, CheckIcon, XIcon } from "lucide-react"
import { toast } from "sonner"
import type { BookRequest } from "@/lib/api"
import { useBooks, useBookRequests, useDeclineBookRequest, useFulfillBookRequest } from "@/lib/queries"
import { errorMessage, timeAgo } from "@/lib/format"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

export function RequestsView() {
  const requests = useBookRequests()
  const decline = useDeclineBookRequest()
  const [fulfilling, setFulfilling] = useState<BookRequest | null>(null)

  return (
    <>
      <PageHeading title="Requests." description="Books readers have asked for. Upload the book in Library first, then fulfill the request with it." />
      {requests.isPending ? (
        <div className="grid gap-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-20 rounded-xl" />)}</div>
      ) : requests.isError ? (
        <EmptyState icon={<BookMarkedIcon />} title="Requests could not be loaded">{errorMessage(requests.error, "Something went wrong.")}</EmptyState>
      ) : requests.data.length === 0 ? (
        <EmptyState icon={<BookMarkedIcon />} title="No open requests">Readers can ask for a book that isn't in the library yet, and it will show up here.</EmptyState>
      ) : (
        <ol className="grid divide-y rounded-xl border bg-background">
          {requests.data.map((request) => (
            <li key={request.id} className="flex flex-wrap items-center gap-4 px-4 py-3.5 animate-in fade-in duration-300">
              <div className="min-w-0 flex-1">
                <p className="font-semibold text-navy">{request.title}</p>
                <p className="truncate text-sm text-muted-foreground">
                  {[request.author, request.requestedByEmail].filter(Boolean).join(" · requested by ")}
                </p>
              </div>
              <span className="text-xs text-muted-foreground tabular-nums">{timeAgo(request.createdAt)}</span>
              <Button variant="outline" size="sm" onClick={() => setFulfilling(request)}><CheckIcon data-icon="inline-start" />Fulfill</Button>
              <Button
                variant="ghost" size="sm" disabled={decline.isPending}
                onClick={() => decline.mutate(request.id, { onSuccess: () => toast.success("Request declined."), onError: (e) => toast.error(errorMessage(e, "Could not decline the request.")) })}
              >
                <XIcon data-icon="inline-start" />Decline
              </Button>
            </li>
          ))}
        </ol>
      )}
      <FulfillDialog request={fulfilling} onClose={() => setFulfilling(null)} />
    </>
  )
}

// Fulfilling requires picking a book that's already in the library -- there is no path that
// approves a request without pointing it at real, uploaded content.
function FulfillDialog({ request, onClose }: { request: BookRequest | null; onClose: () => void }) {
  const { books, loading } = useBooks()
  const fulfill = useFulfillBookRequest()
  const [query, setQuery] = useState("")

  const matches = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return []
    return books.filter((b) => b.title.toLowerCase().includes(needle) || b.authors.some((a) => a.toLowerCase().includes(needle))).slice(0, 8)
  }, [books, query])

  const choose = (bookId: string) => {
    if (!request) return
    fulfill.mutate(
      { id: request.id, bookId },
      {
        onSuccess: () => { toast.success("Request fulfilled."); close() },
        onError: (e) => toast.error(errorMessage(e, "Upload the book in Library first, then fulfill this request with it.")),
      },
    )
  }
  const close = () => { setQuery(""); fulfill.reset(); onClose() }

  return (
    <Dialog open={request !== null} onOpenChange={(next) => { if (!next) close() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="text-2xl font-bold text-navy">Fulfill "{request?.title}"</DialogTitle>
          <DialogDescription>Search for the book once it's uploaded in Library, then select it to fulfill this request.</DialogDescription>
        </DialogHeader>
        <label htmlFor="fulfill-search" className="sr-only">Search the library by title or author</label>
        <Input id="fulfill-search" autoFocus placeholder="Search the library by title or author…" value={query} onChange={(e) => setQuery(e.target.value)} disabled={loading} />
        <div className="grid max-h-72 gap-1 overflow-y-auto">
          {query.trim() && matches.length === 0 && (
            <p className="px-1 py-3 text-sm text-muted-foreground">No uploaded book matches. Upload it in Library first.</p>
          )}
          {matches.map((book) => (
            <button
              key={book.id} type="button" disabled={fulfill.isPending} onClick={() => choose(book.id)}
              className="flex flex-col items-start gap-0.5 rounded-lg px-3 py-2 text-left transition-colors hover:bg-mist disabled:opacity-60"
            >
              <span className="font-medium text-navy">{book.title}</span>
              {book.authors.length > 0 && <span className="text-xs text-muted-foreground">{book.authors.join(", ")}</span>}
            </button>
          ))}
        </div>
        <DialogFooter><Button type="button" variant="outline" onClick={close}>Cancel</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
