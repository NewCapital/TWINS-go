import i18n from './config';
import { NAMESPACES } from './config';
import { isLanguageSupported } from './languages';

// Track loaded languages to avoid duplicate loading
const loadedLanguages = new Set<string>(['en']);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type LanguageLoader = () => Promise<{ default: Record<string, any> }>;

/**
 * Dynamically import language files
 * Each language has its own bundle that's loaded on demand
 */
const languageLoaders: Record<string, LanguageLoader> = {
  bg: () => import('./locales/bg/index'),
  ca: () => import('./locales/ca/index'),
  cs: () => import('./locales/cs/index'),
  da: () => import('./locales/da/index'),
  de: () => import('./locales/de/index'),
  en_US: () => import('./locales/en_US/index'),
  eo: () => import('./locales/eo/index'),
  es: () => import('./locales/es/index'),
  es_ES: () => import('./locales/es_ES/index'),
  fi: () => import('./locales/fi/index'),
  fr_FR: () => import('./locales/fr_FR/index'),
  hi_IN: () => import('./locales/hi_IN/index'),
  hr: () => import('./locales/hr/index'),
  hr_HR: () => import('./locales/hr_HR/index'),
  it: () => import('./locales/it/index'),
  ja: () => import('./locales/ja/index'),
  ko_KR: () => import('./locales/ko_KR/index'),
  lt_LT: () => import('./locales/lt_LT/index'),
  nl: () => import('./locales/nl/index'),
  pl: () => import('./locales/pl/index'),
  pt: () => import('./locales/pt/index'),
  pt_BR: () => import('./locales/pt_BR/index'),
  ro_RO: () => import('./locales/ro_RO/index'),
  ru: () => import('./locales/ru/index'),
  sk: () => import('./locales/sk/index'),
  sv: () => import('./locales/sv/index'),
  tr: () => import('./locales/tr/index'),
  uk: () => import('./locales/uk/index'),
  vi: () => import('./locales/vi/index'),
  zh_CN: () => import('./locales/zh_CN/index'),
  zh_TW: () => import('./locales/zh_TW/index'),
};

/**
 * Load a language's translation files dynamically
 * @param lang Language code to load
 * @returns Promise that resolves when language is loaded
 */
export async function loadLanguage(lang: string): Promise<void> {
  // Already loaded
  if (loadedLanguages.has(lang)) {
    await i18n.changeLanguage(lang);
    return;
  }

  // Check if supported
  if (!isLanguageSupported(lang)) {
    console.warn(`Language ${lang} not supported, falling back to English`);
    await i18n.changeLanguage('en');
    return;
  }

  // Get loader
  const loader = languageLoaders[lang];
  if (!loader) {
    console.warn(`No loader for language ${lang}, falling back to English`);
    await i18n.changeLanguage('en');
    return;
  }

  try {
    // Load all namespaces for this language
    const module = await loader();
    const translations = module.default;

    // Add each namespace to i18n
    for (const ns of Object.values(NAMESPACES)) {
      const nsTranslations = translations[ns];
      if (nsTranslations) {
        i18n.addResourceBundle(lang, ns, nsTranslations, true, true);
      }
    }

    loadedLanguages.add(lang);
    await i18n.changeLanguage(lang);
  } catch (error) {
    console.error(`Failed to load language ${lang}:`, error);
    await i18n.changeLanguage('en');
  }
}

/**
 * Get current language code
 */
export function getCurrentLanguage(): string {
  return i18n.language || 'en';
}

/**
 * Check if a language is already loaded
 */
export function isLanguageLoaded(lang: string): boolean {
  return loadedLanguages.has(lang);
}
