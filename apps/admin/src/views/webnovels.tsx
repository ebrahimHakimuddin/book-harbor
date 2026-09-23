import { useState, type FormEvent } from "react"
import { CheckIcon, Loader2Icon, NewspaperIcon, PlusIcon, RefreshCwIcon, SearchIcon, TriangleAlertIcon, XIcon } from "lucide-react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api, type Webnovel } from "@/lib/api"
import { keys } from "@/lib/queries"
import { errorMessage, timeAgo } from "@/lib/format"
import { Cover } from "@/components/cover"
import { useConfirm } from "@/components/confirm"
import { EmptyState, PageHeading } from "@/components/page-heading"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

const WEBNOVELS = ["webnovels"] as const

/**
 * Serialized novels from novelarchive.cc: following one builds it into an EPUB in the library,
 * and ongoing ones gain new chapters on a schedule. Readers' apps fetch the updated file.
 */
export function WebnovelsView() {
  const followed = useQuery({
    queryKey: WEBNOVELS,
    queryFn: api.webnovels,
    // Poll while the worker is busy, so progress moves on screen.
    refetchInterval: (query) => (query.state.data?.items.some((n) => n.progress || (!n.bookId && !n.error)) ? 2000 : false),
  })
  return (
    <>
      <PageHeading title="Web novels." description="Follow a novel from novelarchive.cc to add it to the library as a book. Ongoing ones get new chapters automatically." />
      <div className="grid gap-8">
        <Search />
        <section className="grid gap-3">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <h2 className="text-2xl font-bold text-navy">Following</h2>
            {followed.data && <Schedule hours={followed.data.intervalHours} />}
          </div>
          {followed.isPending ? <Skeleton className="h-24 rounded-xl" /> : followed.isError ? (
            <EmptyState icon={<NewspaperIcon />} title="Web novels could not be loaded">{errorMessage(followed.error, "Something went wrong.")}</EmptyState>
          ) : followed.data.items.length === 0 ? (
            <EmptyState icon={<NewspaperIcon />} title="Nothing followed yet">Search above and follow a novel to add it to the library.</EmptyState>
          ) : (
            <ol className="grid divide-y overflow-hidden rounded-xl border bg-background">
              {followed.data.items.map((novel) => <FollowedRow key={novel.sourceId} novel={novel} />)}
            </ol>
          )}
        </section>
      </div>
    </>
  )
}

function Search() {
  const client = useQueryClient()
  const [query, setQuery] = useState("")
  const [justFollowed, setJustFollowed] = useState<Set<string>>(new Set())
  const search = useMutation({ mutationFn: (q: string) => api.searchWebnovels(q) })
  const follow = useMutation({
    mutationFn: api.followWebnovel,
    onSuccess: (_, id) => {
      toast.success("Following. The book appears in the library once its chapters are fetched.")
      setJustFollowed((ids) => new Set(ids).add(id))
      void client.invalidateQueries({ queryKey: WEBNOVELS })
    },
    onError: (e) => toast.error(errorMessage(e, "Could not follow that novel.")),
  })
  const submit = (event: FormEvent) => { event.preventDefault(); if (query.trim()) search.mutate(query.trim()) }
  return (
    <section className="grid gap-3">
      <form onSubmit={submit} className="flex max-w-2xl gap-2">
        <div className="relative flex-1">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <label htmlFor="webnovel-search" className="sr-only">Search novelarchive.cc</label>
          <Input id="webnovel-search" className="pl-9" placeholder="Search novelarchive.cc by title" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <Button type="submit" disabled={search.isPending || !query.trim()}>{search.isPending && <Loader2Icon className="animate-spin" />}Search</Button>
      </form>
      {search.isError && <p role="alert" className="text-sm text-destructive">{errorMessage(search.error, "The search failed.")}</p>}
      {search.data && (
        <ul className="grid gap-2 sm:grid-cols-2" aria-live="polite">
          {search.data.items.length === 0 && <li className="text-sm text-muted-foreground">No novels match.</li>}
          {search.data.items.map((novel) => (
            <li key={novel.id} className="flex gap-3 rounded-xl border bg-background p-3">
              <Cover book={{ title: novel.title, coverUrl: novel.coverUrl }} className="w-14 shrink-0" />
              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold text-navy">{novel.title}</p>
                <p className="truncate text-sm text-muted-foreground">{novel.author}</p>
                <p className="mt-1 text-xs text-muted-foreground">{novel.chapters} chapters · {novel.ongoing ? "ongoing" : "completed"}</p>
              </div>
              {novel.followed || justFollowed.has(novel.id) ? (
                <Badge variant="secondary" className="self-start"><CheckIcon />Following</Badge>
              ) : (
                <Button size="sm" variant="outline" className="self-start" disabled={follow.isPending} onClick={() => follow.mutate(novel.id)}>
                  <PlusIcon data-icon="inline-start" />Follow
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function FollowedRow({ novel }: { novel: Webnovel }) {
  const client = useQueryClient()
  const confirm = useConfirm()
  const refresh = () => void client.invalidateQueries({ queryKey: WEBNOVELS })
  const sync = useMutation({ mutationFn: () => api.syncWebnovel(novel.sourceId), onSuccess: refresh, onError: (e) => toast.error(errorMessage(e, "Could not start a sync.")) })
  const unfollow = useMutation({
    mutationFn: () => api.unfollowWebnovel(novel.sourceId),
    onSuccess: () => { refresh(); void client.invalidateQueries({ queryKey: keys.books }) },
    onError: (e) => toast.error(errorMessage(e, "Could not unfollow.")),
  })
  const stop = async () => {
    if (await confirm({
      title: `Stop following "${novel.title}"?`, action: "Stop following",
      description: "No more chapters will be added. The book stays in the library as it is now.",
    })) unfollow.mutate()
  }
  const status = novel.progress
    ? <span className="flex items-center gap-1.5"><Loader2Icon className="size-3.5 animate-spin" />{novel.progress}</span>
    : !novel.bookId && !novel.error
      ? <span className="flex items-center gap-1.5"><Loader2Icon className="size-3.5 animate-spin" />Waiting to fetch chapters…</span>
      : !novel.bookId ? null
      : <span>{novel.chapters} chapters{novel.checkedAt ? ` · checked ${timeAgo(novel.checkedAt)}` : ""}</span>
  return (
    <li className="flex flex-wrap items-center gap-4 px-4 py-4 sm:flex-nowrap">
      <Cover book={{ title: novel.title, coverUrl: novel.coverUrl }} className="w-12 shrink-0 shadow-sm" />
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold text-navy">{novel.title}</p>
        {novel.author && <p className="truncate text-sm text-muted-foreground">{novel.author}</p>}
        <div className="mt-1 text-xs text-muted-foreground" role="status">{status}</div>
        {novel.error && <p className="mt-1 flex items-start gap-1.5 text-xs text-destructive"><TriangleAlertIcon className="mt-px size-3.5 shrink-0" />{novel.error}</p>}
      </div>
      <Badge variant={novel.ongoing ? "secondary" : "outline"}>{novel.ongoing ? "Ongoing" : "Completed"}</Badge>
      <div className="flex gap-2">
        <Button variant="ghost" size="sm" disabled={unfollow.isPending} onClick={stop}><XIcon data-icon="inline-start" />Unfollow</Button>
        <Button variant="outline" size="sm" disabled={sync.isPending || !!novel.progress} onClick={() => sync.mutate()}><RefreshCwIcon data-icon="inline-start" />Sync now</Button>
      </div>
    </li>
  )
}

/** How often ongoing novels are checked; 0 turns automatic updates off. */
function Schedule({ hours }: { hours: number }) {
  const client = useQueryClient()
  const [draft, setDraft] = useState<string | null>(null)
  const save = useMutation({
    mutationFn: (value: string) => api.updateSettings({ "webnovels.syncHours": value }),
    onSuccess: (settings) => {
      client.setQueryData(["settings"], settings)
      setDraft(null)
      void client.invalidateQueries({ queryKey: WEBNOVELS })
      toast.success("Update schedule saved.")
    },
    onError: (e) => toast.error(errorMessage(e, "Could not save the schedule.")),
  })
  const value = draft ?? String(hours)
  return (
    <form className="flex items-center gap-2 text-sm" onSubmit={(e) => { e.preventDefault(); save.mutate(value.trim()) }}>
      <label htmlFor="webnovel-hours" className="text-muted-foreground">Check ongoing novels every</label>
      <Input id="webnovel-hours" type="number" min={0} max={720} className="h-8 w-20" value={value} onChange={(e) => setDraft(e.target.value)} />
      <span className="text-muted-foreground">hours{hours === 0 && draft === null ? " (off)" : ""}</span>
      {draft !== null && draft !== String(hours) && <Button type="submit" size="sm" disabled={save.isPending}>Save</Button>}
    </form>
  )
}
