export type Role = "admin" | "reader"

export interface User {
  id: string
  displayName: string
  email: string
  role: Role
  disabled: boolean
  createdAt: string
}

export interface Session {
  accessToken: string
  refreshToken: string
  user: User
}

export interface Edition {
  id: string
  format: "epub" | "pdf"
  mediaType: string
  originalFilename: string
  byteLength: number
  sha256: string
  createdAt: string
  contentUrl: string
}

export interface Book {
  id: string
  title: string
  subtitle: string
  description: string
  authors: string[]
  coverUrl: string
  source?: { provider: string; id: string }
  createdAt: string
  updatedAt: string
  editions: Edition[]
}

export interface BookPage {
  items: Book[]
  nextCursor?: string
}

export interface BookUpdate {
  title?: string
  subtitle?: string
  description?: string
  authors?: string[]
  coverUrl?: string
  source?: { provider: string; id: string }
}

export interface Instance {
  name: string
  version: string
  setupRequired: boolean
  formats: string[]
  invitesEnabled: boolean
  metadataEnabled: boolean
}

export interface Candidate {
  provider: string
  id: string
  title: string
  subtitle: string
  description: string
  authors: string[]
  coverUrl: string
  publishedDate: string
}

export interface BookRequest {
  id: string
  title: string
  author: string
  coverUrl: string
  status: "open" | "fulfilled" | "declined"
  fulfilledBookId: string | null
  createdAt: string
  requestedByEmail?: string
}

export interface AuditEntry {
  id: number
  actorEmail: string
  action: string
  targetType: string
  targetId: string
  summary: string
  createdAt: string
}

export class APIError extends Error {
  status: number
  code: string | undefined
  constructor(status: number, code: string | undefined, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

// ---- session storage -------------------------------------------------------

const STORAGE_KEY = "bookharbor.admin.session"
const listeners = new Set<() => void>()

function readStoredSession(): Session | null {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null")
  } catch {
    return null
  }
}

let current: Session | null = readStoredSession()

export const sessionStore = {
  get: () => current,
  subscribe(listener: () => void) {
    listeners.add(listener)
    return () => listeners.delete(listener)
  },
  set(session: Session | null) {
    current = session
    try {
      if (session) localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
      else localStorage.removeItem(STORAGE_KEY)
    } catch {
      /* storage can be unavailable; the in-memory session still works */
    }
    listeners.forEach((listener) => listener())
  },
}

let refreshing: Promise<boolean> | null = null

// One refresh at a time: a burst of 401s must not spend the refresh token twice.
function refreshSession(): Promise<boolean> {
  if (!current?.refreshToken) return Promise.resolve(false)
  refreshing ??= (async () => {
    try {
      const response = await fetch("/api/v1/sessions/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refreshToken: current?.refreshToken }),
      })
      if (!response.ok) throw new Error("refresh failed")
      sessionStore.set(await response.json())
      return true
    } catch {
      sessionStore.set(null)
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

// ---- requests --------------------------------------------------------------

interface RequestOptions {
  method?: string
  body?: unknown
  auth?: boolean
  headers?: Record<string, string>
}

async function send(path: string, options: RequestOptions): Promise<Response> {
  const build = () => {
    const headers = new Headers(options.headers)
    let body: BodyInit | undefined
    if (options.body instanceof FormData || options.body instanceof Blob) {
      body = options.body
    } else if (options.body !== undefined) {
      headers.set("Content-Type", "application/json")
      body = JSON.stringify(options.body)
    }
    if (options.auth !== false && current) headers.set("Authorization", `Bearer ${current.accessToken}`)
    return fetch(path, { method: options.method ?? "GET", headers, body })
  }
  let response = await build()
  if (response.status === 401 && options.auth !== false && (await refreshSession())) response = await build()
  return response
}

async function toError(response: Response): Promise<APIError> {
  const payload = await response.json().catch(() => ({}))
  return new APIError(response.status, payload.code, payload.message ?? `Request failed with status ${response.status}.`)
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const response = await send(path, options)
  if (!response.ok) throw await toError(response)
  if (response.status === 204) return undefined as T
  return response.json()
}

// XHR, because fetch cannot report upload progress.
function upload<T>(path: string, body: FormData, onProgress?: (fraction: number) => void): Promise<T> {
  const attempt = () =>
    new Promise<{ status: number; text: string }>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open("POST", path)
      if (current) xhr.setRequestHeader("Authorization", `Bearer ${current.accessToken}`)
      xhr.upload.onprogress = (event) => event.lengthComputable && onProgress?.(event.loaded / event.total)
      xhr.onload = () => resolve({ status: xhr.status, text: xhr.responseText })
      xhr.onerror = () => reject(new APIError(0, "network", "The server could not be reached."))
      xhr.send(body)
    })
  return (async () => {
    let result = await attempt()
    if (result.status === 401 && (await refreshSession())) result = await attempt()
    const payload = result.text ? JSON.parse(result.text) : {}
    if (result.status < 200 || result.status >= 300) {
      throw new APIError(result.status, payload.code, payload.message ?? `Request failed with status ${result.status}.`)
    }
    return payload as T
  })()
}

const enc = encodeURIComponent

export const api = {
  instance: () => request<Instance>("/api/v1/instance", { auth: false }),
  me: () => request<User>("/api/v1/me"),
  updateSelf: (body: { displayName?: string; currentPassword?: string; newPassword?: string }) =>
    request<User>("/api/v1/me", { method: "PATCH", body }),
  async login(email: string, password: string) {
    const session = await request<Session>("/api/v1/sessions", { method: "POST", body: { email, password }, auth: false })
    if (session.user.role !== "admin") throw new APIError(403, "forbidden", "This console is available to administrators only.")
    sessionStore.set(session)
    return session
  },
  bootstrap: (body: { displayName: string; email: string; password: string }) =>
    request<User>("/api/v1/bootstrap", { method: "POST", body, auth: false }),
  async logout() {
    try {
      await request("/api/v1/sessions/current", { method: "DELETE" })
    } catch {
      /* signing out locally still succeeds */
    }
    sessionStore.set(null)
  },

  books: (cursor?: string) => request<BookPage>(`/api/v1/books?limit=100${cursor ? `&cursor=${enc(cursor)}` : ""}`),
  importBook(file: File, title: string, onProgress?: (fraction: number) => void) {
    const data = new FormData()
    data.append("file", file)
    if (title.trim()) data.append("title", title.trim())
    return upload<Book>("/api/v1/books", data, onProgress)
  },
  updateBook: (id: string, body: BookUpdate) => request<Book>(`/api/v1/books/${enc(id)}`, { method: "PATCH", body }),
  deleteBook: (id: string) => request<void>(`/api/v1/books/${enc(id)}`, { method: "DELETE" }),
  putCover: (id: string, file: File) =>
    request<Book>(`/api/v1/books/${enc(id)}/cover`, { method: "PUT", body: file, headers: { "Content-Type": file.type } }),
  deleteCover: (id: string) => request<Book>(`/api/v1/books/${enc(id)}/cover`, { method: "DELETE" }),
  addEdition(id: string, file: File, onProgress?: (fraction: number) => void) {
    const data = new FormData()
    data.append("file", file)
    return upload<Book>(`/api/v1/books/${enc(id)}/editions`, data, onProgress)
  },
  searchMetadata: (query: string) => request<{ items: Candidate[] }>(`/api/v1/admin/metadata/search?q=${enc(query)}`),

  readers: () => request<{ items: User[] }>("/api/v1/admin/users"),
  createReader: (body: { displayName: string; email: string; password?: string; invite?: boolean }) =>
    request<User>("/api/v1/admin/users", { method: "POST", body }),
  updateReader: (id: string, body: { role?: Role; disabled?: boolean; password?: string }) =>
    request<User>(`/api/v1/admin/users/${enc(id)}`, { method: "PATCH", body }),
  deleteReader: (id: string) => request<void>(`/api/v1/admin/users/${enc(id)}`, { method: "DELETE" }),

  audit: (limit = 100) => request<{ items: AuditEntry[] }>(`/api/v1/admin/audit?limit=${limit}`),

  bookRequests: () => request<{ items: BookRequest[] }>("/api/v1/admin/book-requests"),
  fulfillBookRequest: (id: string, bookId: string) =>
    request<void>(`/api/v1/admin/book-requests/${enc(id)}/fulfill`, { method: "POST", body: { bookId } }),
  declineBookRequest: (id: string) => request<void>(`/api/v1/admin/book-requests/${enc(id)}/decline`, { method: "POST" }),

  // Uploaded covers need the bearer token, so they are fetched and shown as blob URLs.
  async coverBlobUrl(path: string) {
    const response = await send(path, {})
    if (!response.ok) throw await toError(response)
    return URL.createObjectURL(await response.blob())
  },
  async exportArchive() {
    const response = await send("/api/v1/admin/export", {})
    if (!response.ok) throw await toError(response)
    return response.blob()
  },
}
