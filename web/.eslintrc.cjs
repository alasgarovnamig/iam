module.exports = {
  root: true,
  env: { browser: true, es2022: true },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
  ],
  ignorePatterns: ['dist', 'node_modules', '.eslintrc.cjs', 'src/api/schema.ts'],
  parser: '@typescript-eslint/parser',
  plugins: ['react-refresh'],
  rules: {
    'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
  },
  overrides: [
    {
      // Playwright specs and fixtures are Node code, not React. The
      // react-hooks rule mistakes Playwright's `use` fixture callback for a
      // React hook, and fast-refresh has no meaning here.
      files: ['e2e/**/*.ts', 'playwright.config.ts', 'vite.config.ts'],
      env: { browser: false, node: true },
      rules: {
        'react-hooks/rules-of-hooks': 'off',
        'react-refresh/only-export-components': 'off',
      },
    },
    {
      // These modules deliberately export a component alongside the constant
      // or helper that belongs with it (the password policy next to the page
      // that enforces it; formatters next to the presenters that use them).
      // Fast refresh degrades to a full reload for them — an acceptable trade
      // for keeping related code together.
      files: ['src/components/common.tsx', 'src/pages/ForcePasswordChangePage.tsx'],
      rules: { 'react-refresh/only-export-components': 'off' },
    },
  ],
};
