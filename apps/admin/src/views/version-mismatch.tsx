import { RefreshCwIcon } from "lucide-react"
import { APP_VERSION } from "@/lib/api"
import { BrandMark } from "@/components/brand"
import { Button } from "@/components/ui/button"

/**
 * Shown instead of the console when it and the server are on different versions. The server
 * serves its own console, so this almost always means the server was upgraded while this tab
 * was open, and reloading fixes it.
 */
export function VersionMismatch({ serverVersion }: { serverVersion?: string }) {
  return (
    <main className="grid min-h-dvh place-items-center bg-sand px-4">
      <div role="alert" className="grid max-w-md justify-items-center gap-4 rounded-2xl border bg-background p-8 text-center shadow-sm">
        <BrandMark className="h-14" />
        <h1 className="text-2xl font-bold text-navy">This page is out of date</h1>
        <p className="text-sm text-muted-foreground">
          The server {serverVersion ? `is on version ${serverVersion}` : "was updated"}, but this page is version {APP_VERSION}.
          They have to match, so nothing can be changed until the page reloads.
        </p>
        <dl className="grid w-full grid-cols-2 gap-2 rounded-lg bg-mist p-3 text-sm">
          <dt className="text-muted-foreground">This page</dt><dd className="font-medium tabular-nums">{APP_VERSION}</dd>
          <dt className="text-muted-foreground">Server</dt><dd className="font-medium tabular-nums">{serverVersion ?? "newer"}</dd>
        </dl>
        <Button onClick={() => location.reload()}><RefreshCwIcon />Reload</Button>
      </div>
    </main>
  )
}
