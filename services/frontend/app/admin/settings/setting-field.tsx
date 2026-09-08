'use client'

import { Chip, Input, Select, SelectItem, Switch, Textarea } from '@heroui/react'
import type { Field } from '@/lib/settings'

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
      {field.restart ? (
        <Chip size="sm" variant="flat" color="warning">
          needs a restart
        </Chip>
      ) : null}
    </span>
  )

  const description = (
    <>
      {field.help ? <span className="block">{field.help}</span> : null}
      {field.env ? (
        <span className="block text-default-400">
          was <code className="font-mono text-tiny">{field.env}</code>
        </span>
      ) : null}
    </>
  )

  switch (field.kind) {
    case 'boolean':
      return (
        <div className="flex items-start justify-between gap-6 py-2">
          <div className="flex flex-col">
            <span className="text-small">{label}</span>
            <span className="text-tiny text-default-500">{description}</span>
          </div>
          <Switch
            isSelected={Boolean(value)}
            onValueChange={next => onChange(field.key, next)}
            aria-label={field.label}
          />
        </div>
      )

    case 'select':
      return (
        <Select
          label={label}
          description={description}
          selectedKeys={value === undefined || value === null ? [] : [String(value)]}
          onSelectionChange={keys => {
            const next = Array.from(keys)[0]
            if (next !== undefined) onChange(field.key, String(next))
          }}
          variant="bordered"
        >
          {(field.options ?? []).map(option => (
            <SelectItem key={option.value}>{option.label}</SelectItem>
          ))}
        </Select>
      )

    case 'text':
    case 'json':
      return (
        <Textarea
          label={label}
          description={description}
          value={value === undefined || value === null ? '' : String(value)}
          onValueChange={next => onChange(field.key, next)}
          minRows={field.kind === 'json' ? 4 : 3}
          variant="bordered"
        />
      )

    case 'password':
      return (
        <Input
          label={label}
          description={description}
          type="password"
          // A secret is never sent back, so the box starts empty and empty
          // means "leave it alone".
          placeholder={field.isSet ? 'set — leave blank to keep, or "-" to clear' : undefined}
          value={value === undefined || value === null ? '' : String(value)}
          onValueChange={next => onChange(field.key, next)}
          variant="bordered"
        />
      )

    case 'number':
      return (
        <Input
          label={label}
          description={description}
          type="number"
          value={value === undefined || value === null ? '' : String(value)}
          onValueChange={next => onChange(field.key, next === '' ? '' : Number(next))}
          variant="bordered"
        />
      )

    default:
      return (
        <Input
          label={label}
          description={description}
          value={value === undefined || value === null ? '' : String(value)}
          onValueChange={next => onChange(field.key, next)}
          variant="bordered"
        />
      )
  }
}
