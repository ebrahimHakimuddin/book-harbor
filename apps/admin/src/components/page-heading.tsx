import type { ReactNode } from "react"

export function PageHeading({ title, description, actions }: { title: string; description: string; actions?: ReactNode }) {
  return (
    <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="text-4xl leading-none font-bold text-navy sm:text-5xl lg:text-6xl">{title}</h1>
        <p className="mt-3 max-w-prose text-muted-foreground">{description}</p>
      </div>
      {actions}
    </div>
  )
}

export function EmptyState({ icon, title, children, action }: { icon: ReactNode; title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-input px-6 py-14 text-center animate-in fade-in zoom-in-95 duration-300">
      <span className="mb-1 grid size-12 place-items-center rounded-full bg-mist text-teal-dark [&_svg]:size-6">{icon}</span>
      <h2 className="text-xl font-bold text-navy">{title}</h2>
      <p className="max-w-sm text-sm text-muted-foreground">{children}</p>
      {action && <div className="mt-3">{action}</div>}
    </div>
  )
}
