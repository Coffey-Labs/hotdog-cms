import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The build goes straight into the Go package, which embeds it, so the
// editor ships as one binary. No inline scripts or styles are emitted: the
// editor serves itself with a strict Content-Security-Policy.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/editor/ui/dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
  },
  server: {
    proxy: { '/api': 'http://127.0.0.1:8190', '/auth': 'http://127.0.0.1:8190' },
  },
})
