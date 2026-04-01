import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import prettierRecommended from 'eslint-plugin-prettier/recommended';
import globals from 'globals';

export default tseslint.config(
  // Global ignores (replaces ignorePatterns)
  { ignores: ['dist/**', 'wailsjs/**', '*.config.js'] },

  // Base ESLint recommended rules
  js.configs.recommended,

  // TypeScript recommended rules
  tseslint.configs.recommended,

  // React recommended + version detection
  {
    ...react.configs.flat.recommended,
    settings: { react: { version: 'detect' } },
  },

  // React 17+ JSX runtime (no need for React import in JSX)
  react.configs.flat['jsx-runtime'],

  // React Hooks recommended rules
  reactHooks.configs.flat.recommended,

  // React Refresh for Vite HMR
  reactRefresh.configs.vite,

  // Project-specific overrides for TS/TSX files
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      globals: {
        ...globals.browser,
      },
    },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      'react/react-in-jsx-scope': 'off',
      'react/prop-types': 'off',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
    },
  },

  // Prettier must be last to disable conflicting formatting rules
  prettierRecommended
);
