import type { ComponentProps } from "react"
import type { LucideIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

/** An icon-only button with a tooltip and an accessible name. */
export function IconAction({ label, icon: Icon, danger, ...props }: { label: string; icon: LucideIcon; danger?: boolean } & Omit<ComponentProps<typeof Button>, "children">) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon" aria-label={label} {...props} className={cn("text-muted-foreground hover:text-navy", danger && "hover:bg-destructive/10 hover:text-destructive")} />}>
        <Icon />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
