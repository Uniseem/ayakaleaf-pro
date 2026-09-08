'use client'

import { Button, Card, CardBody, CardHeader, Link } from '@heroui/react'
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
            <p className="text-small text-default-500">
              {pending} unsaved {pending === 1 ? 'change' : 'changes'}
            </p>
          ) : saved ? (
            <p className="text-small text-success">
              Saved {saved} {saved === 1 ? 'setting' : 'settings'}.
            </p>
          ) : (
            <p className="text-small text-default-500">
              {description.fields.length} settings in {description.sections.length} sections
            </p>
          )}
        </div>
        <Button color="primary" onPress={save} isLoading={busy} isDisabled={pending === 0}>
          Save
        </Button>
      </div>

      <FormError error={error} />

      {needsRestart ? (
        <div className="rounded-medium border border-warning-200 bg-warning-50 px-4 py-3 text-small text-warning-700 dark:bg-warning-50/10">
          One of the changed settings is only read when the site starts. Restart
          it for that one to take effect.
        </div>
      ) : null}

      <nav className="flex flex-wrap gap-x-4 gap-y-2 text-small">
        {description.sections.map(section => (
          <Link key={section.id} href={`#section-${section.id}`} size="sm">
            {section.label}
          </Link>
        ))}
      </nav>

      {description.sections.map(section => {
        const fields = description.fields.filter(field => field.section === section.id)
        if (fields.length === 0) return null
        return (
          <Card key={section.id} id={`section-${section.id}`} shadow="sm">
            <CardHeader className="flex flex-col items-start gap-1 px-6 pt-6">
              <h2 className="text-lg font-medium">{section.label}</h2>
              {section.help ? (
                <p className="text-small text-default-500">{section.help}</p>
              ) : null}
            </CardHeader>
            <CardBody className="gap-5 px-6 pb-6">
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
