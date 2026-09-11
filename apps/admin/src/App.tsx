import { useState, useSyncExternalStore } from "react"
import { sessionStore } from "@/lib/api"
import { useInstance } from "@/lib/queries"
import { BrandMark } from "@/components/brand"
import { EntryScreen } from "@/views/entry"
import { Shell } from "@/views/shell"

export default function App() {
  const session = useSyncExternalStore(sessionStore.subscribe, sessionStore.get)
  const instance = useInstance()
  const [email, setEmail] = useState("")

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
