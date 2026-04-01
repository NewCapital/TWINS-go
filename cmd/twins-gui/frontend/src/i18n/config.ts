import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

// Import all namespaces for English (bundled, always available)
import common_en from './locales/en/common.json';
import wallet_en from './locales/en/wallet.json';
import masternode_en from './locales/en/masternode.json';
import settings_en from './locales/en/settings.json';

// Namespace definitions
export const NAMESPACES = {
  common: 'common',
  wallet: 'wallet',
  masternode: 'masternode',
  settings: 'settings',
} as const;

export type Namespace = (typeof NAMESPACES)[keyof typeof NAMESPACES];

// Initialize with English bundled - other languages lazy-loaded
const resources = {
  en: {
    common: common_en,
    wallet: wallet_en,
    masternode: masternode_en,
    settings: settings_en,
  },
};

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    defaultNS: 'common',
    ns: Object.values(NAMESPACES),

    interpolation: {
      escapeValue: false, // React already escapes
    },

    detection: {
      // Order: localStorage → navigator
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupLocalStorage: 'twins-language',
    },

    react: {
      useSuspense: false, // Avoid loading flicker
    },

    // Development only
    debug: import.meta.env.DEV,
    saveMissing: import.meta.env.DEV,
    missingKeyHandler: (_lngs, ns, key) => {
      if (import.meta.env.DEV) {
        console.warn(`Missing translation: [${ns}] ${key}`);
      }
    },
  });

// After initialization, load language bundles if not English
// This is async but we don't block - translations load in background
if (i18n.language && i18n.language !== 'en') {
  import('./lazyLoader').then(({ loadLanguage }) => {
    loadLanguage(i18n.language).catch((err) => {
      console.error('Failed to load initial language:', err);
    });
  });
}

export default i18n;
