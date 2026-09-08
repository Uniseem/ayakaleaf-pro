import { useCallback, useEffect, useState } from 'react'
import { getJSON, postJSON, getUserFacingMessage } from '@/infrastructure/fetch-json'

// The page is drawn from the catalogue the server sends, so a setting is added
// to the product by adding a row to SiteSettingsCatalogue.mjs and nothing here
// changes.

type Section = { id: string; label: string; help?: string }

type Setting = {
  key: string
  section: string
  type: 'string' | 'text' | 'boolean' | 'number' | 'password' | 'select'
  label: string
  help?: string
  options?: Array<{ value: string; label: string }>
  restart: boolean
  fromEnvironment: boolean
  value?: string | number | boolean
  isSet?: boolean
}

type Catalogue = { sections: Section[]; settings: Setting[] }

export default function SiteSettings() {
  const [catalogue, setCatalogue] = useState<Catalogue | null>(null)
  const [draft, setDraft] = useState<Record<string, unknown>>({})
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: string; text: string } | null>(
    null
  )

  const load = useCallback(async () => {
    try {
      const data = await getJSON<Catalogue>('/admin/settings/api')
      setCatalogue(data)
      setDraft({})
    } catch (error) {
      setMessage({ type: 'danger', text: getUserFacingMessage(error) ?? 'could not load the settings' })
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const change = (key: string, value: unknown) =>
    setDraft(previous => ({ ...previous, [key]: value }))

  const save = async () => {
    if (Object.keys(draft).length === 0) {
      return
    }
    setSaving(true)
    setMessage(null)
    try {
      const result = await postJSON<{ applied: string[]; settings: Catalogue }>(
        '/admin/settings/api',
        { body: { settings: draft } }
      )
      setCatalogue(result.settings)
      setDraft({})
      setMessage({
        type: 'success',
        text: `Saved ${result.applied.length} setting${result.applied.length === 1 ? '' : 's'}.`,
      })
    } catch (error) {
      setMessage({ type: 'danger', text: getUserFacingMessage(error) ?? 'could not save' })
    } finally {
      setSaving(false)
    }
  }

  const sendTestEmail = async () => {
    setMessage(null)
    try {
      const result = await postJSON<{ message: string }>(
        '/admin/settings/test-email',
        { body: {} }
      )
      setMessage({ type: 'success', text: result.message })
    } catch (error) {
      setMessage({ type: 'danger', text: getUserFacingMessage(error) ?? 'could not send' })
    }
  }

  if (!catalogue) {
    return <div className="p-4">Loading…</div>
  }

  const pending = Object.keys(draft).length
  const needsRestart = catalogue.settings.some(
    setting => setting.restart && setting.key in draft
  )

  return (
    <div className="site-settings">
      <div className="d-flex align-items-center justify-content-between my-4">
        <h1 className="h2 m-0">Settings</h1>
        <div className="d-flex gap-2 align-items-center">
          {pending > 0 && (
            <span className="text-muted">
              {pending} unsaved change{pending === 1 ? '' : 's'}
            </span>
          )}
          <button
            className="btn btn-primary"
            onClick={save}
            disabled={saving || pending === 0}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>

      {message && (
        <div className={`alert alert-${message.type}`}>{message.text}</div>
      )}
      {needsRestart && (
        <div className="alert alert-warning">
          One of the changed settings is only read when the site starts. Restart
          the container for it to take effect.
        </div>
      )}

      {catalogue.sections.map(section => {
        const settings = catalogue.settings.filter(
          setting => setting.section === section.id
        )
        if (settings.length === 0) return null
        return (
          <div className="card mb-4" key={section.id}>
            <div className="card-body">
              <h2 className="h4">{section.label}</h2>
              {section.help && <p className="text-muted">{section.help}</p>}
              {settings.map(setting => (
                <Field
                  key={setting.key}
                  setting={setting}
                  draft={draft}
                  onChange={change}
                />
              ))}
              {section.id === 'email' && (
                <button
                  className="btn btn-secondary btn-sm mt-2"
                  onClick={sendTestEmail}
                  type="button"
                >
                  Send a test email to myself
                </button>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

function Field({
  setting,
  draft,
  onChange,
}: {
  setting: Setting
  draft: Record<string, unknown>
  onChange: (key: string, value: unknown) => void
}) {
  const id = `setting-${setting.key}`
  const dirty = setting.key in draft
  const value = dirty ? draft[setting.key] : setting.value

  const label = (
    <label htmlFor={id} className="form-label">
      {setting.label}
      {setting.restart && (
        <span className="badge bg-secondary ms-2">needs a restart</span>
      )}
    </label>
  )

  if (setting.type === 'boolean') {
    return (
      <div className="form-check mb-3">
        <input
          className="form-check-input"
          type="checkbox"
          id={id}
          checked={Boolean(value)}
          onChange={event => onChange(setting.key, event.target.checked)}
        />
        <label className="form-check-label" htmlFor={id}>
          {setting.label}
        </label>
        {setting.help && (
          <div className="form-text">{setting.help}</div>
        )}
      </div>
    )
  }

  return (
    <div className="mb-3">
      {label}
      {setting.type === 'text' ? (
        <textarea
          className="form-control"
          id={id}
          rows={3}
          value={String(value ?? '')}
          onChange={event => onChange(setting.key, event.target.value)}
        />
      ) : setting.type === 'select' ? (
        <select
          className="form-select"
          id={id}
          value={String(value ?? '')}
          onChange={event => onChange(setting.key, event.target.value)}
        >
          {(setting.options ?? []).map(option => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : (
        <input
          className="form-control"
          id={id}
          type={setting.type === 'password' ? 'password' : setting.type === 'number' ? 'number' : 'text'}
          value={
            setting.type === 'password'
              ? String(draft[setting.key] ?? '')
              : String(value ?? '')
          }
          placeholder={
            setting.type === 'password' && setting.isSet
              ? 'set — leave blank to keep, or "-" to clear'
              : undefined
          }
          onChange={event => onChange(setting.key, event.target.value)}
        />
      )}
      {setting.help && <div className="form-text">{setting.help}</div>}
    </div>
  )
}
