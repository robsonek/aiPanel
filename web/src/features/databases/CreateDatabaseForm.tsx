import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

type SiteOption = {
  id: number
  domain: string
}

type CreateDatabaseFormProps = {
  sites: SiteOption[]
  availableEngines: Array<'mariadb' | 'postgres'>
  selectedSiteID: number | null
  onSelectSite: (siteID: number) => void
  onCreated: (password: string) => void
}

export function CreateDatabaseForm({
  sites,
  availableEngines,
  selectedSiteID,
  onSelectSite,
  onCreated,
}: CreateDatabaseFormProps) {
  const { t } = useTranslation()
  const [dbName, setDBName] = useState('')
  const [dbEngine, setDBEngine] = useState<'mariadb' | 'postgres' | ''>('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const sanitizeDBName = (value: string) =>
    value.replace(/[^a-zA-Z0-9_]+/g, '_').replace(/_+/g, '_').replace(/^_+|_+$/g, '')

  useEffect(() => {
    if (availableEngines.length === 0) {
      setDBEngine('')
      return
    }
    setDBEngine((prev) => (prev && availableEngines.includes(prev) ? prev : availableEngines[0]))
  }, [availableEngines])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!selectedSiteID) {
      setError(t('databases.errors.siteRequired'))
      return
    }
    const normalizedName = sanitizeDBName(dbName.trim())
    if (!normalizedName) {
      setError(t('databases.errors.createFailed'))
      setSubmitting(false)
      return
    }
    if (!dbEngine) {
      setError(t('databases.errors.engineRequired'))
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      const res = await fetch(`/api/sites/${selectedSiteID}/databases`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ db_name: normalizedName, db_engine: dbEngine }),
      })
      if (!res.ok) {
        const message = await res.text()
        setError(message || t('databases.errors.createFailed'))
        return
      }
      const payload = (await res.json()) as { password: string }
      setDBName('')
      onCreated(payload.password)
    } catch {
      setError(t('errors.network'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form className="grid gap-3 rounded-lg border border-[var(--border-subtle)] p-4" onSubmit={onSubmit}>
      <h3 className="font-heading text-lg">{t('databases.create.title')}</h3>
      <label className="block">
        <span className="mb-1 block text-sm">{t('databases.create.siteLabel')}</span>
        <select
          className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          value={selectedSiteID ?? ''}
          onChange={(e) => onSelectSite(Number(e.target.value))}
        >
          <option value="">{t('databases.create.sitePlaceholder')}</option>
          {sites.map((site) => (
            <option key={site.id} value={site.id}>
              {site.domain}
            </option>
          ))}
        </select>
      </label>

      <label className="block">
        <span className="mb-1 block text-sm">{t('databases.create.dbNameLabel')}</span>
        <input
          className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          value={dbName}
          onChange={(e) => setDBName(sanitizeDBName(e.target.value))}
          placeholder={t('databases.create.dbNamePlaceholder')}
          maxLength={64}
          required
        />
      </label>

      <label className="block">
        <span className="mb-1 block text-sm">{t('databases.create.engineLabel')}</span>
        <select
          className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          value={dbEngine}
          onChange={(e) => setDBEngine(e.target.value as 'mariadb' | 'postgres')}
          disabled={availableEngines.length === 0}
        >
          {availableEngines.length === 0 ? (
            <option value="">{t('databases.create.enginePlaceholder')}</option>
          ) : null}
          {availableEngines.map((engine) => (
            <option key={engine} value={engine}>
              {engine === 'mariadb' ? t('databases.create.engineMariaDB') : t('databases.create.enginePostgreSQL')}
            </option>
          ))}
        </select>
      </label>

      {availableEngines.length === 0 ? (
        <p className="rounded-md border border-[var(--state-warning)]/40 bg-[var(--state-warning)]/10 px-3 py-2 text-sm text-[var(--state-warning)]">
          {t('databases.create.noAvailableEngines')}
        </p>
      ) : null}

      {error ? (
        <p className="rounded-md border border-[var(--state-danger)]/40 bg-[var(--state-danger)]/10 px-3 py-2 text-sm text-[var(--state-danger)]">
          {error}
        </p>
      ) : null}

      <button
        type="submit"
        disabled={submitting || availableEngines.length === 0}
        className="rounded-md bg-[var(--accent-primary)] px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
      >
        {submitting ? t('databases.create.submitting') : t('databases.create.submit')}
      </button>
    </form>
  )
}
