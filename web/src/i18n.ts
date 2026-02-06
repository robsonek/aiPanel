import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './locales/en.json'

const LANGUAGE_STORAGE_KEY = 'aipanel_language'

const canUseLocalStorage = (): boolean =>
  typeof window !== 'undefined' &&
  typeof window.localStorage !== 'undefined' &&
  typeof window.localStorage.getItem === 'function' &&
  typeof window.localStorage.setItem === 'function'

const resolveInitialLanguage = (): string => {
  if (typeof window === 'undefined') {
    return 'en'
  }

  if (canUseLocalStorage()) {
    const savedLanguage = window.localStorage.getItem(LANGUAGE_STORAGE_KEY)
    if (savedLanguage) {
      return savedLanguage
    }
  }

  const browserLanguage =
    typeof window.navigator.language === 'string'
      ? window.navigator.language.split('-')[0]
      : ''
  return browserLanguage || 'en'
}

void i18n.use(initReactI18next).init({
  resources: {
    en: {
      translation: en,
    },
  },
  supportedLngs: ['en'],
  nonExplicitSupportedLngs: true,
  lng: resolveInitialLanguage(),
  fallbackLng: 'en',
  interpolation: {
    escapeValue: false,
  },
})

if (canUseLocalStorage()) {
  i18n.on('languageChanged', (language) => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, language)
  })
}

export default i18n
