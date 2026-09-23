import { readFileSync } from 'node:fs'
import path from 'node:path'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// The Go server embeds the build output and serves it at /admin/.
// The release version lives in /VERSION, shared with the server and the Android app.
const version = readFileSync(path.resolve(import.meta.dirname, '../../VERSION'), 'utf8').trim()

export default defineConfig({
  base: '/admin/',
  define: { __APP_VERSION__: JSON.stringify(version) },
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
  build: {
    outDir: '../server/internal/httpapi/adminui',
    emptyOutDir: true,
  },
  server: {
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
})
