import { APIError } from "@/lib/api"

export const initials = (value?: string) =>
  (value ?? "BH").split(/\s+/).filter(Boolean).slice(0, 2).map((word) => word[0]).join("").toUpperCase()

export function formatBytes(bytes: number) {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" })
export function timeAgo(iso: string) {
  const seconds = (new Date(iso).getTime() - Date.now()) / 1000
  const units: [Intl.RelativeTimeFormatUnit, number][] = [["day", 86400], ["hour", 3600], ["minute", 60]]
  for (const [unit, size] of units) if (Math.abs(seconds) >= size) return relative.format(Math.round(seconds / size), unit)
  return "just now"
}

export const fullDate = (iso: string) => new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso))
export const shortDate = (iso: string) => new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(new Date(iso))

export const errorMessage = (error: unknown, fallback: string) => (error instanceof APIError ? error.message : fallback)

// 4 words from a small list is readable aloud and well above the 12-character minimum.
const WORDS = ["harbor", "lantern", "compass", "anchor", "meadow", "willow", "ember", "cobalt", "juniper", "saffron", "tidal", "quartz", "maple", "falcon", "orchid", "pebble", "summit", "breeze", "copper", "velvet"]
export function generatePassword() {
  const random = crypto.getRandomValues(new Uint32Array(5))
  const words = Array.from(random.slice(0, 4), (n) => WORDS[n % WORDS.length])
  return `${words.join("-")}-${100 + (random[4] % 900)}`
}

export const passwordOk = (value: string) => value.length >= 12
