import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'

type Theme = 'light' | 'dark'

type User = {
  id: number
  email: string
  role: string
}

const navItems = [
  'Dashboard',
  'Sites & Domains',
  'Databases',
  'Updates & Versions',
  'Backup & Restore',
  'Security & Audit',
  'Settings',
]

function App() {
  const [theme, setTheme] = useState<Theme>('light')
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

  const hostStatus = useMemo(() => (user ? 'Host: OK' : 'Host: --'), [user])

  const toggleTheme = () => setTheme((prev) => (prev === 'light' ? 'dark' : 'light'))

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setAuthError(null)

    if (!email.includes('@')) {
      setAuthError('Please enter a valid email address.')
      return
    }
    if (password.length < 10) {
      setAuthError('Password must be at least 10 characters.')
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
        setAuthError('Invalid email or password.')
        return
      }
      const payload = (await res.json()) as { user: User }
      setUser(payload.user)
      setPassword('')
    } catch {
      setAuthError('Network error. Please try again.')
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
        Loading...
      </div>
    )
  }

  if (!user) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-[var(--bg-canvas)] p-6 text-[var(--text-primary)]">
        <section className="w-full max-w-md rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6 shadow-sm">
          <h1 className="font-heading text-2xl">aiPanel</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">Sign in to continue.</p>
          <form className="mt-6 space-y-4" onSubmit={onSubmit}>
            <label className="block">
              <span className="mb-1 block text-sm">Email</span>
              <input
                className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="admin@example.com"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-sm">Password</span>
              <input
                className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••"
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
              {isSubmitting ? 'Signing in...' : 'Sign in'}
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
            Menu
          </button>
          <span className="font-heading text-lg">aiPanel</span>
        </div>
        <div className="hidden items-center gap-3 text-sm text-[var(--text-secondary)] md:flex">
          <span>{hostStatus}</span>
          <button
            type="button"
            className="rounded-md border border-[var(--border-subtle)] px-2 py-1 hover:bg-[var(--bg-canvas)]"
            onClick={toggleTheme}
          >
            {theme === 'light' ? 'Dark' : 'Light'}
          </button>
          <span>{user.email}</span>
          <button
            type="button"
            className="rounded-md border border-[var(--border-subtle)] px-2 py-1 hover:bg-[var(--bg-canvas)]"
            onClick={logout}
          >
            Logout
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
                key={item}
                type="button"
                className="block w-full rounded-md px-3 py-2 text-left text-sm hover:bg-[var(--bg-canvas)]"
                onClick={() => setMobileNavOpen(false)}
              >
                {item}
              </button>
            ))}
          </nav>
          <div className="mt-6 border-t border-[var(--border-subtle)] pt-4 md:hidden">
            <button
              type="button"
              className="mb-2 block w-full rounded-md border border-[var(--border-subtle)] px-3 py-2 text-left text-sm"
              onClick={toggleTheme}
            >
              Theme: {theme}
            </button>
            <button
              type="button"
              className="block w-full rounded-md border border-[var(--border-subtle)] px-3 py-2 text-left text-sm"
              onClick={logout}
            >
              Logout
            </button>
          </div>
        </aside>

        <main className="w-full p-4 md:p-6">
          <section className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6">
            <h2 className="font-heading text-xl">Dashboard</h2>
            <p className="mt-2 text-[var(--text-secondary)]">Welcome, {user.email}</p>
            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <article className="rounded-lg border border-[var(--border-subtle)] p-4">
                <h3 className="text-sm text-[var(--text-secondary)]">CPU</h3>
                <p className="mt-1 font-semibold">43%</p>
              </article>
              <article className="rounded-lg border border-[var(--border-subtle)] p-4">
                <h3 className="text-sm text-[var(--text-secondary)]">RAM</h3>
                <p className="mt-1 font-semibold">61%</p>
              </article>
              <article className="rounded-lg border border-[var(--border-subtle)] p-4">
                <h3 className="text-sm text-[var(--text-secondary)]">Health score</h3>
                <p className="mt-1 font-semibold">92/100</p>
              </article>
            </div>
          </section>
        </main>
      </div>
    </div>
  )
}

export default App
