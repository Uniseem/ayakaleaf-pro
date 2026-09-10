'use client'

import { Badge } from '@/components/ol/badge'
import { OLFormControl, OLFormGroup, OLFormLabel, OLFormText } from '@/components/ol/form-control'
import { Select } from '@/components/ol/select'
import type { Field } from '@/lib/settings'

type Option = { value: string; label: string }

/**
 * One setting, drawn from what the API said about it.
 *
 * Nothing here knows any particular setting's name. That is what keeps adding
 * one to a row in the catalogue rather than a change in two places.
 */
export function SettingField({
  field,
  value,
  onChange,
}: {
  field: Field
  value: unknown
  onChange: (key: string, value: unknown) => void
}) {
  const label = (
    <span className="flex items-center gap-2">
      {field.label}
      {field.restart ? <Badge bg="warning">needs a restart</Badge> : null}
    </span>
  )

  const description = (
    <>
      {field.help ? <span className="block">{field.help}</span> : null}
      {field.env ? (
        <span className="block text-[var(--content-secondary)]">
          was <code className="font-mono text-xs">{field.env}</code>
        </span>
      ) : null}
    </>
  )

  const asText = value === undefined || value === null ? '' : String(value)

  switch (field.kind) {
    case 'boolean':
      return (
        <div className="flex items-start justify-between gap-6 py-2">
          <div className="flex flex-col">
            <span>{label}</span>
            <span className="text-xs text-[var(--content-secondary)]">{description}</span>
          </div>
          <input
            type="checkbox"
            className="form-check-input"
            checked={Boolean(value)}
            onChange={event => onChange(field.key, event.target.checked)}
            aria-label={field.label}
          />
        </div>
      )

    case 'select': {
      const options: Option[] = field.options ?? []
      const selected = options.find(option => option.value === asText) ?? null
      return (
        <OLFormGroup controlId={`setting-${field.key}`}>
          <Select<Option>
            label={label}
            items={options}
            itemToKey={option => option.value}
            itemToString={option => option?.label ?? ''}
            selected={selected}
            onSelectedItemChanged={option => {
              if (option) {
                onChange(field.key, option.value)
              }
            }}
          />
          <OLFormText>{description}</OLFormText>
        </OLFormGroup>
      )
    }

    case 'text':
    case 'json':
      return (
        <OLFormGroup controlId={`setting-${field.key}`}>
          <OLFormLabel>{label}</OLFormLabel>
          <OLFormControl
            as="textarea"
            rows={field.kind === 'json' ? 4 : 3}
            value={asText}
            onChange={event => onChange(field.key, event.target.value)}
          />
          <OLFormText>{description}</OLFormText>
        </OLFormGroup>
      )

    case 'password':
      return (
        <OLFormGroup controlId={`setting-${field.key}`}>
          <OLFormLabel>{label}</OLFormLabel>
          <OLFormControl
            type="password"
            // A secret is never sent back, so the box starts empty and empty
            // means "leave it alone".
            placeholder={field.isSet ? 'set — leave blank to keep, or "-" to clear' : undefined}
            value={asText}
            onChange={event => onChange(field.key, event.target.value)}
          />
          <OLFormText>{description}</OLFormText>
        </OLFormGroup>
      )

    case 'number':
      return (
        <OLFormGroup controlId={`setting-${field.key}`}>
          <OLFormLabel>{label}</OLFormLabel>
          <OLFormControl
            type="number"
            value={asText}
            onChange={event =>
              onChange(field.key, event.target.value === '' ? '' : Number(event.target.value))
            }
          />
          <OLFormText>{description}</OLFormText>
        </OLFormGroup>
      )

    default:
      return (
        <OLFormGroup controlId={`setting-${field.key}`}>
          <OLFormLabel>{label}</OLFormLabel>
          <OLFormControl value={asText} onChange={event => onChange(field.key, event.target.value)} />
          <OLFormText>{description}</OLFormText>
        </OLFormGroup>
      )
  }
}
