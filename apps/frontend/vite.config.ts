import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Source maps are disabled for release builds embedded in the Go binary to
// keep the binary size small.  Set RELEASE=1 (via `make build`) to suppress
// them.  During local `pnpm dev` / `pnpm build` sourcemaps are always enabled
// for a better debugging experience.
const isRelease = process.env['RELEASE'] === '1';

export default defineConfig({
  plugins: [react()],
  resolve: {
    // Force a single copy of React across the pnpm workspace. Prevents Vite from
    // pre-bundling a stale react@18 jsx-runtime alongside react@19 (ReactCurrentDispatcher crash).
    dedupe: ['react', 'react-dom'],
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      // In dev the Vite proxy forwards /api/* to the Go binary.
      // In production the React SPA is served by the same Go binary on the
      // same origin, so all /api/* fetches are same-origin (no proxy needed).
      '/api': {
        target: 'http://127.0.0.1:8787',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: 'dist',
    // Disable source maps for release embeds (RELEASE=1) to keep binary size
    // small.  Enabled for local builds and CI to aid debugging.
    sourcemap: isRelease ? false : true,
  },
});
