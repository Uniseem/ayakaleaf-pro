'use client'

import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { Button } from '@/components/ol/button'
import { useMemo, useState } from 'react'
import { FormError } from '@/components/form-error'
import { saveSettings, type SettingsDescription } from '@/lib/settings'
import { SettingField } from './setting-field'

/**
 * The admin settings.
 *
 * The page is drawn from the catalogue the API sends, so a setting is added to
 * the product by adding a row there and nothing here changes. Only what has
 * been touched is sent, which is what makes an untouched secret keep its
 * value.
 */
export function SettingsForm({ initial }: { initial: SettingsDescription }) {
  const [description, setDescription] = useState(initial)
  const [draft, setDraft] = useState<Record<string, unknown>>({})
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  const pending = Object.keys(draft).length
  const needsRestart = useMemo(
    () => description.fields.some(field => field.restart && field.key in draft),
    [description.fields, draft]
  )

  function change(key: string, value: unknown) {
    setDraft(previous => ({ ...previous, [key]: value }))
    setSaved(null)
  }

  async function save() {
    if (pending === 0) return
    setBusy(true)
    setError(null)
    try {
      const next = await saveSettings(draft)
      setDescription(next)
      setSaved(pending)
      setDraft({})
    } catch (thrown) {
      setError(thrown)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="sticky top-0 z-20 -mx-4 flex items-center justify-between gap-4 bg-background/80 px-4 py-3 backdrop-blur">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
          {pending > 0 ? (
            <p className="text-[var(--content-secondary)]">
              {pending} unsaved {pending === 1 ? 'change' : 'changes'}
            </p>
          ) : saved ? (
            <p className="text-[var(--content-positive)]">
              Saved {saved} {saved === 1 ? 'setting' : 'settings'}.
            </p>
          ) : (
            <p className="text-[var(--content-secondary)]">
              {description.fields.length} settings in {description.sections.length} sections
            </p>
          )}
        </div>
        <Button variant="primary" onClick={save} isLoading={busy} disabled={pending === 0}>
          Save
        </Button>
      </div>

      <FormError error={error} />

      {needsRestart ? (
        <div className="alert alert-warning">
          One of the changed settings is only read when the site starts. Restart
          it for that one to take effect.
        </div>
      ) : null}

      <nav className="flex flex-wrap gap-x-4 gap-y-2">
        {description.sections.map(section => (
          <a key={section.id} href={`#section-${section.id}`}>
            {section.label}
          </a>
        ))}
      </nav>

      {description.sections.map(section => {
        const fields = description.fields.filter(field => field.section === section.id)
        if (fields.length === 0) return null
        return (
          <Card key={section.id} id={`section-${section.id}`}>
            <CardHeader title={section.label} subtitle={section.help} />
            <CardBody className="flex flex-col gap-5">
              {fields.map(field => (
                <SettingField
                  key={field.key}
                  field={field}
                  value={field.key in draft ? draft[field.key] : field.value}
                  onChange={change}
                />
              ))}
            </CardBody>
          </Card>
        )
      })}
    </div>
  )
}
