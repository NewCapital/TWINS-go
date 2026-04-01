/**
 * Supported languages configuration
 * Mapped from legacy Qt wallet twins_*.ts files
 */

export interface LanguageConfig {
  code: string; // ISO 639-1 code
  name: string; // English name
  nativeName: string; // Native name for display
}

/**
 * All 33 languages supported by the legacy Qt wallet
 * Native names displayed in language selector
 */
export const SUPPORTED_LANGUAGES: LanguageConfig[] = [
  { code: 'en', name: 'English', nativeName: 'English' },
  { code: 'bg', name: 'Bulgarian', nativeName: 'Български' },
  { code: 'ca', name: 'Catalan', nativeName: 'Català' },
  { code: 'cs', name: 'Czech', nativeName: 'Čeština' },
  { code: 'da', name: 'Danish', nativeName: 'Dansk' },
  { code: 'de', name: 'German', nativeName: 'Deutsch' },
  { code: 'eo', name: 'Esperanto', nativeName: 'Esperanto' },
  { code: 'es', name: 'Spanish', nativeName: 'Español' },
  { code: 'es_ES', name: 'Spanish (Spain)', nativeName: 'Español (España)' },
  { code: 'fi', name: 'Finnish', nativeName: 'Suomi' },
  { code: 'fr_FR', name: 'French', nativeName: 'Français' },
  { code: 'hi_IN', name: 'Hindi', nativeName: 'हिन्दी' },
  { code: 'hr', name: 'Croatian', nativeName: 'Hrvatski' },
  { code: 'hr_HR', name: 'Croatian (Croatia)', nativeName: 'Hrvatski (Hrvatska)' },
  { code: 'it', name: 'Italian', nativeName: 'Italiano' },
  { code: 'ja', name: 'Japanese', nativeName: '日本語' },
  { code: 'ko_KR', name: 'Korean', nativeName: '한국어' },
  { code: 'lt_LT', name: 'Lithuanian', nativeName: 'Lietuvių' },
  { code: 'nl', name: 'Dutch', nativeName: 'Nederlands' },
  { code: 'pl', name: 'Polish', nativeName: 'Polski' },
  { code: 'pt', name: 'Portuguese', nativeName: 'Português' },
  { code: 'pt_BR', name: 'Portuguese (Brazil)', nativeName: 'Português (Brasil)' },
  { code: 'ro_RO', name: 'Romanian', nativeName: 'Română' },
  { code: 'ru', name: 'Russian', nativeName: 'Русский' },
  { code: 'sk', name: 'Slovak', nativeName: 'Slovenčina' },
  { code: 'sv', name: 'Swedish', nativeName: 'Svenska' },
  { code: 'tr', name: 'Turkish', nativeName: 'Türkçe' },
  { code: 'uk', name: 'Ukrainian', nativeName: 'Українська' },
  { code: 'vi', name: 'Vietnamese', nativeName: 'Tiếng Việt' },
  { code: 'zh_CN', name: 'Chinese (Simplified)', nativeName: '简体中文' },
  { code: 'zh_TW', name: 'Chinese (Traditional)', nativeName: '繁體中文' },
];

/**
 * Map Qt locale codes to i18next codes
 * Qt uses underscores, i18next prefers hyphens but accepts both
 */
export const QT_LANGUAGE_MAP: Record<string, string> = {
  en_US: 'en',
  de_DE: 'de',
  fr_FR: 'fr_FR',
  es_ES: 'es_ES',
  zh_CN: 'zh_CN',
  zh_TW: 'zh_TW',
  ja_JP: 'ja',
  ko_KR: 'ko_KR',
  ru_RU: 'ru',
  pt_BR: 'pt_BR',
  hi_IN: 'hi_IN',
  hr_HR: 'hr_HR',
  lt_LT: 'lt_LT',
  ro_RO: 'ro_RO',
};

/**
 * Get language display name by code
 */
export function getLanguageName(code: string): string {
  const lang = SUPPORTED_LANGUAGES.find((l) => l.code === code);
  return lang?.nativeName || code;
}

/**
 * Check if a language code is supported
 */
export function isLanguageSupported(code: string): boolean {
  return SUPPORTED_LANGUAGES.some((l) => l.code === code);
}
