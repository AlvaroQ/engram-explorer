// ESLint flat config for the frontend islands (TypeScript + React) and the
// plain browser scripts served by the templ UI. Non-type-aware on purpose:
// `pnpm typecheck` already runs tsc, and this keeps `pnpm lint` fast.
import js from '@eslint/js';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

// Leading underscore marks an intentionally unused parameter or catch binding.
const unusedVars = [
  'error',
  { argsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_', varsIgnorePattern: '^_' },
];

export default tseslint.config(
  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/build/**',
      '**/coverage/**',
      'internal/web/dist/**',
      'internal/ui/static/*.min.js',
      'design/**',
      'odd/**',
      '.worktrees/**',
      '.scratch-*/**',
    ],
  },
  {
    files: ['apps/frontend/src/**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: { ...globals.browser },
    },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      '@typescript-eslint/no-unused-vars': unusedVars,
    },
  },
  {
    // Classic (non-module) scripts loaded by the templ layout.
    files: ['internal/ui/static/*.js'],
    extends: [js.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'script',
      // `module` is only read behind a typeof guard (CommonJS export for Node tests).
      globals: { ...globals.browser, htmx: 'readonly', module: 'readonly' },
    },
    rules: {
      'no-unused-vars': unusedVars,
    },
  },
  {
    // Build-tool config files run in Node.
    files: ['eslint.config.js', 'apps/frontend/*.config.{js,ts}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: { ...globals.node },
    },
  },
);
