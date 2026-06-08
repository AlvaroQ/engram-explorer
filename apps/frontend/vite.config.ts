import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

// Source maps are disabled for release builds embedded in the Go binary to
// keep the binary size small.  Set RELEASE=1 (via `make build`) to suppress
// them.  During local `pnpm dev` / `pnpm build` sourcemaps are always enabled
// for a better debugging experience.
const isRelease = process.env['RELEASE'] === '1';

// ---------------------------------------------------------------------------
// Island entries
// ---------------------------------------------------------------------------
// Each island is a self-contained React bundle that gets mounted by loader.ts
// at runtime.  Add one entry per island here; the output path will be
// dist/islands/<name>.js (stable — no hash, no fingerprint).
//
// To add a new island:
//   1. Create src/islands/<name>.tsx exporting mount(el, props).
//   2. Add an entry below: islandName: resolve(__dirname, 'src/islands/<name>.tsx')
//   3. Run `pnpm build` — dist/islands/<name>.js will be emitted.
//   4. Use @island("<name>", data) in your templ page.
const islandEntries: Record<string, string> = {
  'islands/type-breakdown': resolve(__dirname, 'src/islands/type-breakdown.tsx'),
  'islands/activity-by-project-chart': resolve(__dirname, 'src/islands/activity-by-project-chart.tsx'),
  'islands/project-activity-chart': resolve(__dirname, 'src/islands/project-activity-chart.tsx'),
  'islands/brain-preview-card': resolve(__dirname, 'src/islands/brain-preview-card.tsx'),
  'islands/brain': resolve(__dirname, 'src/islands/brain.tsx'),
  'islands/loader': resolve(__dirname, 'src/islands/loader.ts'),
};

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
      // In dev the Vite proxy forwards /api/* to the Go binary so islands
      // can fetch data without CORS issues during local development.
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
    rollupOptions: {
      // Islands are imported dynamically by loader.ts and must keep their
      // named export `mount()`. Without this, Rollup tree-shakes the export
      // off the entry chunk (entries default to exports-only stripping) and
      // the loader throws "Island X does not export a mount() function".
      preserveEntrySignatures: 'strict',
      input: {
        // Island entries — each becomes dist/islands/<name>.js
        ...islandEntries,
      },
      output: {
        // Islands get stable, hash-free file names so templ pages can
        // reference them by predictable URL (/islands/<name>.js).
        // The SPA entry keeps the default Vite fingerprinted name.
        entryFileNames: (chunk) => {
          if (chunk.name.startsWith('islands/')) {
            // e.g. "islands/type-breakdown" → "islands/type-breakdown.js"
            return `${chunk.name}.js`;
          }
          return 'assets/[name]-[hash].js';
        },
        // Shared chunks and CSS keep fingerprinted names (SPA only cares).
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
});
