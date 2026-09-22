import { useEffect } from "react"
import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from "@tanstack/react-query"
import { api, sessionStore, type Book, type BookPage, type BookUpdate, type Role, type User } from "@/lib/api"

export const keys = {
  instance: ["instance"] as const,
  books: ["books"] as const,
  readers: ["readers"] as const,
  audit: ["audit"] as const,
  bookRequests: ["bookRequests"] as const,
}

export const useInstance = () => useQuery({ queryKey: keys.instance, queryFn: api.instance, retry: false })

// Loads every page: search and format filters run in the browser over the full catalog.
export function useBooks() {
  const query = useInfiniteQuery({
    queryKey: keys.books,
    queryFn: ({ pageParam }) => api.books(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page: BookPage) => page.nextCursor,
  })
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = query
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])
  const books = query.data?.pages.flatMap((page) => page.items) ?? []
  return { ...query, books, loading: query.isPending || hasNextPage }
}

// Replace a book everywhere it is cached, without refetching the whole catalog.
function patchBook(client: ReturnType<typeof useQueryClient>, book: Book) {
  client.setQueryData<InfiniteData<BookPage>>(keys.books, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((b) => (b.id === book.id ? book : b)) })) },
  )
}

function useAuditRefresh() {
  const client = useQueryClient()
  return () => client.invalidateQueries({ queryKey: keys.audit })
}

export function useImportBook() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: (v: { file: File; title: string; onProgress?: (f: number) => void }) => api.importBook(v.file, v.title, v.onProgress),
    onSuccess: (book) => {
      client.setQueryData<InfiniteData<BookPage>>(keys.books, (data) =>
        data ? { ...data, pages: data.pages.map((page, i) => (i === 0 ? { ...page, items: [book, ...page.items] } : page)) } : data,
      )
      void audit()
    },
  })
}

export function useUpdateBook() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: (v: { id: string; update: BookUpdate }) => api.updateBook(v.id, v.update),
    onSuccess: (book) => { patchBook(client, book); void audit() },
  })
}

export function useDeleteBook() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: (id: string) => api.deleteBook(id),
    onSuccess: (_, id) => {
      client.setQueryData<InfiniteData<BookPage>>(keys.books, (data) =>
        data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.filter((b) => b.id !== id) })) },
      )
      void audit()
    },
  })
}

export function useCoverMutations() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  const done = (book: Book) => {
    patchBook(client, book)
    void client.invalidateQueries({ queryKey: ["cover"] })
    void audit()
  }
  return {
    upload: useMutation({ mutationFn: (v: { id: string; file: File }) => api.putCover(v.id, v.file), onSuccess: done }),
    remove: useMutation({ mutationFn: (id: string) => api.deleteCover(id), onSuccess: done }),
  }
}

export function useAddEdition() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: (v: { id: string; file: File; onProgress?: (f: number) => void }) => api.addEdition(v.id, v.file, v.onProgress),
    onSuccess: (book) => { patchBook(client, book); void audit() },
  })
}

export const useMetadataSearch = () => useMutation({ mutationFn: (q: string) => api.searchMetadata(q) })

export const useReaders = () => useQuery({ queryKey: keys.readers, queryFn: async () => (await api.readers()).items })

function setReaders(client: ReturnType<typeof useQueryClient>, fn: (users: User[]) => User[]) {
  client.setQueryData<User[]>(keys.readers, (users) => (users ? fn(users) : users))
}

export function useCreateReader() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: api.createReader,
    onSuccess: (user) => { setReaders(client, (users) => [...users, user]); void audit() },
  })
}

export function useUpdateReader() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; role?: Role; disabled?: boolean; password?: string }) => api.updateReader(id, body),
    onSuccess: (user) => { setReaders(client, (users) => users.map((u) => (u.id === user.id ? user : u))); void audit() },
  })
}

// Updates the signed-in admin's own display name and/or password (requires currentPassword
// whenever newPassword is set), and refreshes the cached session so the header reflects it
// immediately without forcing a re-login.
export function useUpdateSelf() {
  return useMutation({
    mutationFn: api.updateSelf,
    onSuccess: (user) => {
      const session = sessionStore.get()
      if (session) sessionStore.set({ ...session, user })
    },
  })
}

export function useDeleteReader() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return useMutation({
    mutationFn: (id: string) => api.deleteReader(id),
    onSuccess: (_, id) => { setReaders(client, (users) => users.filter((u) => u.id !== id)); void audit() },
  })
}

export const useAudit = () => useQuery({ queryKey: keys.audit, queryFn: async () => (await api.audit(100)).items })

// Cover images uploaded to this server are private, so they load through the API client.
// Keyed by URL alone: editing other fields must not refetch the image. Replacing the
// cover invalidates this key (see useCoverMutations).
export function useCoverSrc(url: string) {
  const isPrivate = url.startsWith("/")
  const query = useQuery({
    queryKey: ["cover", url],
    queryFn: () => api.coverBlobUrl(url),
    enabled: isPrivate,
    staleTime: Infinity,
    gcTime: 10 * 60_000,
    retry: false,
  })
  return isPrivate ? query.data : url || undefined
}

export const useBookRequests = (status: "open" | "resolved" = "open") =>
  useQuery({ queryKey: [...keys.bookRequests, status], queryFn: async () => (await api.bookRequests(status)).items })

function useResolveBookRequest() {
  const client = useQueryClient()
  const audit = useAuditRefresh()
  return () => {
    void client.invalidateQueries({ queryKey: keys.bookRequests })
    void audit()
  }
}

export function useFulfillBookRequest() {
  const resolved = useResolveBookRequest()
  return useMutation({
    mutationFn: (v: { id: string; bookId: string }) => api.fulfillBookRequest(v.id, v.bookId),
    onSuccess: resolved,
  })
}

export function useDeclineBookRequest() {
  const resolved = useResolveBookRequest()
  return useMutation({ mutationFn: (id: string) => api.declineBookRequest(id), onSuccess: resolved })
}
