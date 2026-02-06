import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CreateSiteForm } from './CreateSiteForm'

type Site = {
  id: number
  domain: string
  php_version: string
  status: string
  created_at: string
}

export function SitesPage() {
  const { t } = useTranslation()
  const [sites, setSites] = useState<Site[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadSites = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/api/sites', { credentials: 'include' })
      if (!res.ok) {
        throw new Error(t('sites.errors.loadFailed'))
      }
      const payload = (await res.json()) as { sites: Site[] }
      setSites(payload.sites ?? [])
    } catch {
      setError(t('sites.errors.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void loadSites()
  }, [loadSites])

  const deleteSite = async (site: Site) => {
    const confirmed = window.confirm(t('sites.delete.confirm', { domain: site.domain }))
    if (!confirmed) {
      return
    }
    try {
      const res = await fetch(`/api/sites/${site.id}`, {
        method: 'DELETE',
        credentials: 'include',
      })
      if (!res.ok) {
        throw new Error()
      }
      setSites((prev) => prev.filter((item) => item.id !== site.id))
    } catch {
      setError(t('sites.errors.deleteFailed'))
    }
  }

  return (
    <section className="grid gap-4">
      <CreateSiteForm onCreated={() => void loadSites()} />

      <article className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-4 md:p-6">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-heading text-xl">{t('sites.title')}</h2>
          <span className="text-sm text-[var(--text-secondary)]">
            {t('sites.count', { count: sites.length })}
          </span>
        </div>

        {error ? (
          <p className="mt-4 rounded-md border border-[var(--state-danger)]/40 bg-[var(--state-danger)]/10 px-3 py-2 text-sm text-[var(--state-danger)]">
            {error}
          </p>
        ) : null}

        {loading ? (
          <div className="mt-4 grid gap-3">
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
          </div>
        ) : sites.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--text-secondary)]">{t('sites.empty')}</p>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full min-w-[640px] text-left text-sm">
              <thead>
                <tr className="border-b border-[var(--border-subtle)] text-[var(--text-secondary)]">
                  <th className="px-2 py-2 font-medium">{t('sites.table.domain')}</th>
                  <th className="px-2 py-2 font-medium">{t('sites.table.phpVersion')}</th>
                  <th className="px-2 py-2 font-medium">{t('sites.table.status')}</th>
                  <th className="px-2 py-2 font-medium">{t('sites.table.created')}</th>
                  <th className="px-2 py-2 font-medium">{t('sites.table.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {sites.map((site) => (
                  <tr key={site.id} className="border-b border-[var(--border-subtle)]/60">
                    <td className="px-2 py-3">{site.domain}</td>
                    <td className="px-2 py-3">{site.php_version}</td>
                    <td className="px-2 py-3">{site.status}</td>
                    <td className="px-2 py-3">{new Date(site.created_at).toLocaleString()}</td>
                    <td className="px-2 py-3">
                      <button
                        type="button"
                        className="rounded-md border border-[var(--state-danger)]/40 px-2 py-1 text-xs text-[var(--state-danger)] hover:bg-[var(--state-danger)]/10"
                        onClick={() => void deleteSite(site)}
                      >
                        {t('common.delete')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </article>
    </section>
  )
}
