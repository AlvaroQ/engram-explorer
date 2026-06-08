import type { Config } from 'tailwindcss';

export default {
  content: [
    './index.html',
    './src/**/*.{ts,tsx}',
    // Server-rendered templ pages live outside the SPA but share the same
    // Tailwind utility classes — scan them so their classes are emitted.
    '../../internal/ui/**/*.templ',
  ],
  darkMode: ['class'],
  theme: {
    extend: {
      colors: {
        bg: 'hsl(var(--bg))',
        surface: 'hsl(var(--surface))',
        'surface-2': 'hsl(var(--surface-2))',
        border: 'hsl(var(--border))',
        fg: 'hsl(var(--fg))',
        'fg-muted': 'hsl(var(--fg-muted))',
        accent: 'hsl(var(--accent))',
        ok: 'hsl(var(--ok))',
        warn: 'hsl(var(--warn))',
        fail: 'hsl(var(--fail))',
      },
      borderRadius: {
        lg: '0.75rem',
        md: '0.5rem',
        sm: '0.375rem',
      },
      fontFamily: {
        sans: [
          'ui-sans-serif',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Roboto',
          'sans-serif',
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'monospace'],
      },
    },
  },
  plugins: [],
} satisfies Config;
