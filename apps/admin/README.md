# BookHarbor admin console

React + TypeScript + Tailwind + [shadcn/ui](https://ui.shadcn.com), with
[TanStack Query](https://tanstack.com/query) for server state. The Go server serves
it at `/admin/`.

```sh
npm install
npm run dev     # http://localhost:5173/admin/, proxies /api to 127.0.0.1:8080
npm run build   # type-checks, then writes ../server/internal/httpapi/adminui
```

The Go binary embeds that build output, so **commit it** after changing the UI:
`go build` and `go test` then work without Node. The Docker image rebuilds it from
source.

- `src/lib/api.ts` typed API client: session storage, single-flight token refresh,
  upload progress.
- `src/lib/queries.ts` every TanStack Query hook, and the only place cache keys live.
- `src/components/ui/` shadcn components (add more with `npx shadcn@latest add`).
- `src/index.css` holds the theme: the BookHarbor palette with Literata and Inter.
