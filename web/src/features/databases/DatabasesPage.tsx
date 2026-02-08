import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CreateDatabaseForm } from './CreateDatabaseForm'

type Site = {
  id: number
  domain: string
}

type DatabaseEngine = 'mariadb' | 'postgres'

type Database = {
  id: number
  site_id: number
  db_name: string
  db_user: string
  db_engine: string
  created_at: string
}

export function DatabasesPage() {
  const { t } = useTranslation()
  const [sites, setSites] = useState<Site[]>([])
  const [availableEngines, setAvailableEngines] = useState<DatabaseEngine[]>([])
  const [selectedSiteID, setSelectedSiteID] = useState<number | null>(null)
  const [databases, setDatabases] = useState<Database[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [newPassword, setNewPassword] = useState<string | null>(null)

  const loadSites = useCallback(async () => {
    const res = await fetch('/api/sites', { credentials: 'include' })
    if (!res.ok) {
      throw new Error()
    }
    const payload = (await res.json()) as { sites: Site[] }
    const nextSites = payload.sites ?? []
    setSites(nextSites)
    if (nextSites.length > 0) {
      setSelectedSiteID((prev) => prev ?? nextSites[0].id)
    } else {
      setSelectedSiteID(null)
    }
  }, [])

  const loadAvailableEngines = useCallback(async () => {
    const res = await fetch('/api/databases/engines', { credentials: 'include' })
    if (!res.ok) {
      throw new Error()
    }
    const payload = (await res.json()) as { engines: string[] }
    const engines = (payload.engines ?? []).filter((engine): engine is DatabaseEngine =>
      engine === 'mariadb' || engine === 'postgres',
    )
    setAvailableEngines(engines)
  }, [])

  const loadDatabases = useCallback(
    async (siteID: number) => {
      setLoading(true)
      setError(null)
      try {
        const res = await fetch(`/api/sites/${siteID}/databases`, { credentials: 'include' })
        if (!res.ok) {
          throw new Error()
        }
        const payload = (await res.json()) as { databases: Database[] }
        setDatabases(payload.databases ?? [])
      } catch {
        setError(t('databases.errors.loadFailed'))
      } finally {
        setLoading(false)
      }
    },
    [t],
  )

  useEffect(() => {
    const run = async () => {
      setError(null)
      try {
        await Promise.all([loadSites(), loadAvailableEngines()])
      } catch {
        setError(t('databases.errors.loadInitFailed'))
        setLoading(false)
      }
    }
    void run()
  }, [loadAvailableEngines, loadSites, t])

  useEffect(() => {
    if (!selectedSiteID) {
      setDatabases([])
      setLoading(false)
      return
    }
    void loadDatabases(selectedSiteID)
  }, [loadDatabases, selectedSiteID])

  const sitesByID = useMemo(() => new Map(sites.map((site) => [site.id, site.domain])), [sites])

  const onDelete = async (item: Database) => {
    const confirmed = window.confirm(t('databases.delete.confirm', { name: item.db_name }))
    if (!confirmed) {
      return
    }
    try {
      const res = await fetch(`/api/databases/${item.id}`, {
        method: 'DELETE',
        credentials: 'include',
      })
      if (!res.ok) {
        throw new Error()
      }
      setDatabases((prev) => prev.filter((row) => row.id !== item.id))
    } catch {
      setError(t('databases.errors.deleteFailed'))
    }
  }

  return (
    <section className="grid gap-4">
      <CreateDatabaseForm
        sites={sites}
        availableEngines={availableEngines}
        selectedSiteID={selectedSiteID}
        onSelectSite={setSelectedSiteID}
        onCreated={(password) => {
          setNewPassword(password)
          if (selectedSiteID) {
            void loadDatabases(selectedSiteID)
          }
        }}
      />

      <article className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-4 md:p-6">
        <h2 className="font-heading text-xl">{t('databases.title')}</h2>

        {newPassword ? (
          <p className="mt-4 rounded-md border border-[var(--state-warning)]/40 bg-[var(--state-warning)]/10 px-3 py-2 text-sm text-[var(--state-warning)]">
            {t('databases.passwordOnce', { password: newPassword })}
          </p>
        ) : null}

        {error ? (
          <p className="mt-4 rounded-md border border-[var(--state-danger)]/40 bg-[var(--state-danger)]/10 px-3 py-2 text-sm text-[var(--state-danger)]">
            {error}
          </p>
        ) : null}

        {!selectedSiteID ? (
          <p className="mt-4 text-sm text-[var(--text-secondary)]">{t('databases.noSites')}</p>
        ) : loading ? (
          <div className="mt-4 grid gap-3">
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
            <div className="h-10 animate-pulse rounded-md bg-[var(--bg-canvas)]" />
          </div>
        ) : databases.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--text-secondary)]">{t('databases.empty')}</p>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full min-w-[700px] text-left text-sm">
              <thead>
                <tr className="border-b border-[var(--border-subtle)] text-[var(--text-secondary)]">
                  <th className="px-2 py-2 font-medium">{t('databases.table.name')}</th>
                  <th className="px-2 py-2 font-medium">{t('databases.table.user')}</th>
                  <th className="px-2 py-2 font-medium">{t('databases.table.engine')}</th>
                  <th className="px-2 py-2 font-medium">{t('databases.table.site')}</th>
                  <th className="px-2 py-2 font-medium">{t('databases.table.created')}</th>
                  <th className="px-2 py-2 font-medium">{t('databases.table.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {databases.map((item) => (
                  <tr key={item.id} className="border-b border-[var(--border-subtle)]/60">
                    <td className="px-2 py-3">{item.db_name}</td>
                    <td className="px-2 py-3">{item.db_user}</td>
                    <td className="px-2 py-3">{item.db_engine}</td>
                    <td className="px-2 py-3">{sitesByID.get(item.site_id) || '-'}</td>
                    <td className="px-2 py-3">{new Date(item.created_at).toLocaleString()}</td>
                    <td className="px-2 py-3">
                      <button
                        type="button"
                        className="rounded-md border border-[var(--state-danger)]/40 px-2 py-1 text-xs text-[var(--state-danger)] hover:bg-[var(--state-danger)]/10"
                        onClick={() => void onDelete(item)}
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
