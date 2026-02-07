import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

type CreateSiteFormProps = {
  onCreated: () => void
}

export function CreateSiteForm({ onCreated }: CreateSiteFormProps) {
  const { t } = useTranslation()
  const [domain, setDomain] = useState('')
  const [phpVersion, setPHPVersion] = useState('8.5')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const res = await fetch('/api/sites', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          domain: domain.trim().toLowerCase(),
          php_version: phpVersion,
        }),
      })
      if (!res.ok) {
        const message = await res.text()
        setError(message || t('sites.errors.createFailed'))
        return
      }
      setDomain('')
      setPHPVersion('8.5')
      onCreated()
    } catch {
      setError(t('errors.network'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form className="grid gap-3 rounded-lg border border-[var(--border-subtle)] p-4" onSubmit={onSubmit}>
      <h3 className="font-heading text-lg">{t('sites.create.title')}</h3>
      <label className="block">
        <span className="mb-1 block text-sm">{t('sites.create.domainLabel')}</span>
        <input
          className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
          placeholder={t('sites.create.domainPlaceholder')}
          required
        />
      </label>
      <label className="block">
        <span className="mb-1 block text-sm">{t('sites.create.phpVersionLabel')}</span>
        <select
          className="w-full rounded-md border border-[var(--border-subtle)] bg-[var(--bg-canvas)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus-ring)]"
          value={phpVersion}
          onChange={(e) => setPHPVersion(e.target.value)}
        >
          <option value="8.5">8.5</option>
        </select>
      </label>
      {error ? (
        <p className="rounded-md border border-[var(--state-danger)]/40 bg-[var(--state-danger)]/10 px-3 py-2 text-sm text-[var(--state-danger)]">
          {error}
        </p>
      ) : null}
      <button
        type="submit"
        disabled={submitting}
        className="rounded-md bg-[var(--accent-primary)] px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
      >
        {submitting ? t('sites.create.submitting') : t('sites.create.submit')}
      </button>
    </form>
  )
}
