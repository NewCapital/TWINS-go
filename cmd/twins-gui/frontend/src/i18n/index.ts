/**
 * i18n module exports
 * Import this in main.tsx to initialize translations
 */
export { default as i18n } from './config';
export { NAMESPACES, type Namespace } from './config';
export { SUPPORTED_LANGUAGES, getLanguageName, isLanguageSupported } from './languages';
export type { LanguageConfig } from './languages';
export { loadLanguage, getCurrentLanguage, isLanguageLoaded } from './lazyLoader';
