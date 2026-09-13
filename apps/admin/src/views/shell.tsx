import { useEffect, useState } from "react"
import { ArchiveIcon, BookOpenIcon, HistoryIcon, LogOutIcon, UsersIcon, type LucideIcon } from "lucide-react"
import { useQueryClient } from "@tanstack/react-query"
import { api, type Session } from "@/lib/api"
import { useBooks, useReaders } from "@/lib/queries"
import { initials } from "@/lib/format"
import { cn } from "@/lib/utils"
import { Wordmark } from "@/components/brand"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { LibraryView } from "@/views/library"
import { ReadersView } from "@/views/readers"
import { ActivityView } from "@/views/activity"
import { BackupView } from "@/views/backup"

const VIEWS = ["library", "readers", "activity", "backup"] as const
type View = (typeof VIEWS)[number]

const NAV: { view: View; label: string; icon: LucideIcon }[] = [
  { view: "library", label: "Library", icon: BookOpenIcon },
  { view: "readers", label: "Readers", icon: UsersIcon },
  { view: "activity", label: "Activity", icon: HistoryIcon },
  { view: "backup", label: "Backup", icon: ArchiveIcon },
]

// The view lives in the URL hash so a refresh keeps you where you were.
function useView(): [View, (view: View) => void] {
  const read = (): View => (VIEWS.find((v) => location.hash === `#/${v}`) ?? "library")
  const [view, setView] = useState(read)
  useEffect(() => {
    const onHash = () => setView(read())
    addEventListener("hashchange", onHash)
    return () => removeEventListener("hashchange", onHash)
  }, [])
  return [view, (next) => { location.hash = `/${next}` }]
}

export function Shell({ session, instanceName }: { session: Session; instanceName: string }) {
  const client = useQueryClient()
  const [view, setView] = useView()
  const books = useBooks()
  const readers = useReaders()
  const counts: Partial<Record<View, number | undefined>> = {
    library: books.loading ? undefined : books.books.length,
    readers: readers.data?.length,
  }

  useEffect(() => { document.title = `${instanceName} administration` }, [instanceName])

  const signOut = async () => { await api.logout(); client.clear() }

  return (
    <div className="min-h-dvh bg-background">
      <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-lg focus:bg-navy focus:px-4 focus:py-2 focus:text-white">Skip to content</a>
      <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-7 lg:h-[72px]">
        <a href="#/library" className="mr-auto lg:w-[244px]" aria-label="BookHarbor administration home"><Wordmark /></a>
        <div className="hidden text-right leading-tight sm:block">
          <div className="text-sm font-semibold text-navy">{instanceName}</div>
          <div className="text-xs text-muted-foreground">Self-hosted</div>
        </div>
        <Avatar aria-hidden><AvatarFallback className="bg-cyan/25 font-heading font-bold text-navy">{initials(session.user.displayName)}</AvatarFallback></Avatar>
        <Button variant="outline" onClick={signOut}><LogOutIcon data-icon="inline-start" />Sign out</Button>
      </header>

      <div className="lg:grid lg:grid-cols-[244px_minmax(0,1fr)]">
        <nav aria-label="Administration" className="sticky top-16 z-20 flex gap-1 overflow-x-auto border-b bg-background px-3 py-2 lg:top-[72px] lg:h-[calc(100dvh-72px)] lg:flex-col lg:gap-1 lg:overflow-visible lg:border-r lg:border-b-0 lg:px-5 lg:py-8">
          <p className="mb-2 hidden px-3 text-[0.7rem] font-extrabold tracking-widest text-muted-foreground uppercase lg:block">Manage</p>
          {NAV.map(({ view: id, label, icon: Icon }) => {
            const active = view === id
            return (
              <button key={id} type="button" onClick={() => setView(id)} aria-current={active ? "page" : undefined}
                className={cn(
                  "group relative flex shrink-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-semibold text-foreground transition-colors outline-none hover:bg-mist focus-visible:ring-3 focus-visible:ring-ring/60",
                  active && "bg-accent text-navy",
                )}>
                <span aria-hidden className={cn("absolute top-2 bottom-2 left-0 hidden w-[3px] origin-center scale-y-0 rounded-full bg-teal transition-transform lg:block", active && "scale-y-100")} />
                <Icon className={cn("size-[18px] text-muted-foreground transition-all group-hover:translate-x-0.5", active && "text-teal")} />
                <span>{label}</span>
                {counts[id] !== undefined && <span className="ml-auto text-xs font-medium text-muted-foreground tabular-nums">{counts[id]}</span>}
              </button>
            )
          })}
          <div className="mt-auto hidden px-3 pb-2 lg:block">
            <svg viewBox="0 0 44 14" className="mb-3 h-3.5 w-11 fill-none" strokeWidth="2.4" strokeLinecap="round" aria-hidden>
              <path d="M2 5c5-5 9-5 14 0s9 5 14 0 9-5 12-2" stroke="#2e7d7a" /><path d="M2 11c5-5 9-5 14 0s9 5 14 0 9-5 12-2" stroke="#7fb3c3" />
            </svg>
            <p className="font-heading text-[0.95rem] leading-snug text-muted-foreground italic">A brighter tomorrow,<br />one book at a time.</p>
          </div>
        </nav>

        <main id="main" tabIndex={-1} className="min-w-0 px-4 py-7 outline-none sm:px-8 lg:px-14 lg:py-14">
          <div key={view} className="mx-auto max-w-[1320px] animate-in fade-in slide-in-from-bottom-1 duration-200">
            {view === "library" && <LibraryView />}
            {view === "readers" && <ReadersView currentUserId={session.user.id} />}
            {view === "activity" && <ActivityView />}
            {view === "backup" && <BackupView />}
          </div>
        </main>
      </div>
    </div>
  )
}
