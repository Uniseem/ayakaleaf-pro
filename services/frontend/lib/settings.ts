/** The admin settings, as the API describes them. */

import { api } from './api'

export type Kind =
  | 'string'
  | 'text'
  | 'boolean'
  | 'number'
  | 'password'
  | 'select'
  | 'json'

export type Option = { value: string; label: string }

export type Section = {
  id: string
  label: string
  help?: string
}

export type Field = {
  key: string
  section: string
  kind: Kind
  label: string
  help?: string
  options?: Option[]
  env?: string
  restart?: boolean
  secret?: boolean
  default?: unknown
  value?: unknown
  isSet?: boolean
}

export type SettingsDescription = {
  sections: Section[]
  fields: Field[]
}

export function describeSettings(
  headers?: Record<string, string>
): Promise<SettingsDescription> {
  return api<SettingsDescription>('/api/admin/settings', { headers })
}

export function saveSettings(
  values: Record<string, unknown>
): Promise<SettingsDescription> {
  return api<SettingsDescription>('/api/admin/settings', {
    method: 'POST',
    body: { values },
  })
}

/** The words this person has told the spell checker to accept. */
export function learnedWords(): Promise<string[]> {
  return api<{ words: string[] }>('/api/spelling/learned').then(a => a.words)
}

export function learnWord(word: string): Promise<string[]> {
  return api<{ words: string[] }>('/api/spelling/learn', {
    method: 'POST',
    body: { word },
  }).then(a => a.words)
}

export function unlearnWord(word: string): Promise<string[]> {
  return api<{ words: string[] }>('/api/spelling/unlearn', {
    method: 'POST',
    body: { word },
  }).then(a => a.words)
}
