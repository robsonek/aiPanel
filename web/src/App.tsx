import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabasesPage } from './features/databases/DatabasesPage'
import { SitesPage } from './features/sites/SitesPage'
import './i18n'

type Theme = 'light' | 'dark'
type Page = 'dashboard' | 'sites' | 'databases' | 'updates' | 'backup' | 'security' | 'settings'

type User = {
  id: number
  email: string
  role: string
}

const navItems = [
  { page: 'dashboard', labelKey: 'nav.dashboard' },
  { page: 'sites', labelKey: 'nav.sites' },
  { page: 'databases', labelKey: 'nav.databases' },
  { page: 'updates', labelKey: 'nav.updates' },
  { page: 'backup', labelKey: 'nav.backup' },
  { page: 'security', labelKey: 'nav.security' },
  { page: 'settings', labelKey: 'nav.settings' },
] as const

function App() {
  const { t } = useTranslation()
  const [theme, setTheme] = useState<Theme>('light')
  const [activePage, setActivePage] = useState<Page>('dashboard')
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [user, setUser] = useState<User | null>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [authError, setAuthError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [loadingSession, setLoadingSession] = useState(true)

  useEffect(() => {
    const canUseStorage =
      typeof window !== 'undefined' &&
      typeof window.localStorage !== 'undefined' &&
      typeof window.localStorage.getItem === 'function'
    const saved = canUseStorage ? window.localStorage.getItem('aipanel_theme') : null
    if (saved === 'dark' || saved === 'light') {
      setTheme(saved)
      return
    }
    if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      setTheme('dark')
    }
  }, [])

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    if (
      typeof window !== 'undefined' &&
      typeof window.localStorage !== 'undefined' &&
      typeof window.localStorage.setItem === 'function'
    ) {
      window.localStorage.setItem('aipanel_theme', theme)
    }
  }, [theme])

  useEffect(() => {
    const loadSession = async () => {
      try {
        const res = await fetch('/api/auth/me', { credentials: 'include' })
        if (!res.ok) {
          setUser(null)
          return
        }
        const payload = (await res.json()) as { user: User }
        setUser(payload.user)
      } catch {
        setUser(null)
      } finally {
        setLoadingSession(false)
      }
    }
    void loadSession()
  }, [])

  const hostStatus = useMemo(
    () => (user ? t('topbar.hostOk') : t('topbar.hostUnknown')),
    [t, user],
  )

  const toggleTheme = () => setTheme((prev) => (prev === 'light' ? 'dark' : 'light'))

  const pageContent = useMemo(() => {
    if (activePage === 'sites') {
      return <SitesPage />
    }
    if (activePage === 'databases') {
      return <DatabasesPage />
    }
    if (activePage === 'dashboard') {
      return (
        <section className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6">
          <h2 className="font-heading text-xl">{t('dashboard.title')}</h2>
          <p className="mt-2 text-[var(--text-secondary)]">{t('dashboard.welcome', { user: user?.email })}</p>
          <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <article className="rounded-lg border border-[var(--border-subtle)] p-4">
              <h3 className="text-sm text-[var(--text-secondary)]">{t('dashboard.cards.cpu')}</h3>
              <p className="mt-1 font-semibold">43%</p>
            </article>
            <article className="rounded-lg border border-[var(--border-subtle)] p-4">
              <h3 className="text-sm text-[var(--text-secondary)]">{t('dashboard.cards.ram')}</h3>
              <p className="mt-1 font-semibold">61%</p>
            </article>
            <article className="rounded-lg border border-[var(--border-subtle)] p-4">
              <h3 className="text-sm text-[var(--text-secondary)]">{t('dashboard.cards.health')}</h3>
              <p className="mt-1 font-semibold">92/100</p>
            </article>
          </div>
        </section>
      )
    }
    return (
      <section className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6">
        <h2 className="font-heading text-xl">{t(`nav.${activePage}`)}</h2>
        <p className="mt-2 text-sm text-[var(--text-secondary)]">{t('app.comingSoon')}</p>
      </section>
    )
  }, [activePage, t, user?.email])

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setAuthError(null)

    if (!email.includes('@')) {
      setAuthError(t('auth.validation.emailInvalid'))
      return
    }
    if (password.length < 10) {
      setAuthError(t('auth.validation.passwordShort'))
      return
    }

    setIsSubmitting(true)
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })
      if (!res.ok) {
        setAuthError(t('errors.invalidCredentials'))
        return
      }
      const payload = (await res.json()) as { user: User }
      setUser(payload.user)
      setPassword('')
    } catch {
      setAuthError(t('errors.network'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const logout = async () => {
    await fetch('/api/auth/logout', {
      method: 'POST',
      credentials: 'include',
    })
    setUser(null)
    setMobileNavOpen(false)
  }

  if (loadingSession) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-canvas)] text-[var(--text-primary)]">
        {t('app.loading')}
      </div>
    )
  }

  if (!user) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-[var(--bg-canvas)] p-6 text-[var(--text-primary)]">
        <section className="w-full max-w-md rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6 shadow-sm">
          <h1 className="font-heading text-2xl">{t('app.name')}</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">{t('auth.subtitle')}</p>
          <form className="mt-6 space-y-4" onSubmit={onSubmit}>
            <label className="block">
              <span className="mb-1 block text-sm">{t('auth.email')}</span>
              <input
                className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t('auth.placeholders.email')}
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-sm">{t('auth.password')}</span>
              <input
                className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t('auth.placeholders.password')}
              />
            </label>
            {authError ? (
              <p className="rounded-md border border-[var(--state-danger)]/40 bg-[var(--state-danger)]/10 px-3 py-2 text-sm text-[var(--state-danger)]">
                {authError}
              </p>
            ) : null}
            <button
              className="w-full rounded-md bg-[var(--accent-primary)] px-3 py-2 font-medium text-white hover:opacity-95 disabled:opacity-50"
              type="submit"
              disabled={isSubmitting}
            >
              {isSubmitting ? t('auth.signingIn') : t('auth.signIn')}
            </button>
          </form>
        </section>
      </main>
    )
  }

  return (
    <div className="min-h-screen bg-[var(--bg-canvas)] text-[var(--text-primary)]">
      <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-[var(--border-subtle)] bg-[var(--bg-surface)] px-4">
        <div className="flex items-center gap-3">
          <button
            type="button"
            className="rounded-md border border-[var(--border-subtle)] px-2 py-1 text-sm md:hidden"
            onClick={() => setMobileNavOpen((v) => !v)}
          >
            {t('topbar.menu')}
          </button>
          <span className="font-heading text-lg">{t('app.name')}</span>
        </div>
        <div className="hidden flex-1 justify-center px-4 md:flex">
          <label htmlFor="topbar-search" className="sr-only">
            {t('topbar.searchLabel')}
          </label>
          <input
            id="topbar-search"
            type="search"
            placeholder={t('topbar.searchPlaceholder')}
            className="w-full max-w-xl rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          />
        </div>
        <div className="hidden items-center gap-3 text-sm text-[var(--text-secondary)] md:flex">
          <span>{hostStatus}</span>
          <button
            type="button"
            className="rounded-md border border-[var(--border-subtle)] px-2 py-1 hover:bg-[var(--bg-canvas)]"
            onClick={toggleTheme}
          >
            {theme === 'light' ? t('theme.dark') : t('theme.light')}
          </button>
          <span>{user.email}</span>
          <button
            type="button"
            className="rounded-md border border-[var(--border-subtle)] px-2 py-1 hover:bg-[var(--bg-canvas)]"
            onClick={logout}
          >
            {t('auth.logout')}
          </button>
        </div>
      </header>

      <div className="flex">
        <aside
          className={[
            'fixed inset-y-14 left-0 z-20 w-64 border-r border-[var(--border-subtle)] bg-[var(--bg-surface)] p-4 md:static md:inset-auto md:block md:h-[calc(100vh-3.5rem)]',
            mobileNavOpen ? 'block' : 'hidden',
          ].join(' ')}
        >
          <nav className="space-y-1">
            {navItems.map((item) => (
              <button
                key={item.page}
                type="button"
                className={[
                  'block w-full rounded-md px-3 py-2 text-left text-sm hover:bg-[var(--bg-canvas)]',
                  activePage === item.page ? 'bg-[var(--bg-canvas)]' : '',
                ].join(' ')}
                onClick={() => {
                  setActivePage(item.page)
                  setMobileNavOpen(false)
                }}
              >
                {t(item.labelKey)}
              </button>
            ))}
          </nav>
          <div className="mt-6 border-t border-[var(--border-subtle)] pt-4 md:hidden">
            <button
              type="button"
              className="mb-2 block w-full rounded-md border border-[var(--border-subtle)] px-3 py-2 text-left text-sm"
              onClick={toggleTheme}
            >
              {t('topbar.themeCurrent', {
                theme: t(theme === 'light' ? 'theme.light' : 'theme.dark'),
              })}
            </button>
            <button
              type="button"
              className="block w-full rounded-md border border-[var(--border-subtle)] px-3 py-2 text-left text-sm"
              onClick={logout}
            >
              {t('auth.logout')}
            </button>
          </div>
        </aside>

        <main className="w-full p-4 md:p-6">{pageContent}</main>
      </div>
    </div>
  )
}

export default App
