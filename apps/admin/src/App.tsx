import { useState, useSyncExternalStore } from "react"
import { APP_VERSION, compatibleVersions, sessionStore, versionMismatch } from "@/lib/api"
import { useInstance } from "@/lib/queries"
import { BrandMark } from "@/components/brand"
import { EntryScreen } from "@/views/entry"
import { Shell } from "@/views/shell"
import { VersionMismatch } from "@/views/version-mismatch"

export default function App() {
  const session = useSyncExternalStore(sessionStore.subscribe, sessionStore.get)
  const instance = useInstance()
  const [email, setEmail] = useState("")
  const refused = useSyncExternalStore(versionMismatch.subscribe, versionMismatch.get)

  // A console and server on different versions don't work together: block everything.
  if (refused || (instance.data && !compatibleVersions(instance.data.version, APP_VERSION))) {
    return <VersionMismatch serverVersion={instance.data?.version} />
  }
  if (instance.isPending) {
    return <div className="grid min-h-dvh place-items-center bg-sand"><BrandMark className="h-14 animate-pulse" /></div>
  }
  if (instance.isError || instance.data.setupRequired || !session) {
    return (
      <EntryScreen
        setup={!!instance.data?.setupRequired}
        loadError={instance.isError ? "This BookHarbor server could not be reached." : undefined}
        email={email}
        onEmail={setEmail}
      />
    )
  }
  return <Shell session={session} instanceName={instance.data.name} />
}
