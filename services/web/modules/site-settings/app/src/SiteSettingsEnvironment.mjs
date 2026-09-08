import { SETTINGS } from './SiteSettingsCatalogue.mjs'

// The settings that are delivered as environment variables, worked out in one
// place because two things need exactly the same answer: this process, which
// puts them into process.env before the first module is imported, and the
// startup script, which writes them into the container environment before any
// service starts at all.
//
// Both are needed. A module that begins `if (process.env.X === 'true')` is
// satisfied by the first. Anything read while the settings file itself is
// being evaluated -- and that is most of what the other services read -- is
// already past by then, and only the second reaches it.

/** Turns whatever was stored into the shape the setting is declared as. */
function coerce(definition, value) {
  if (value == null) {
    return definition.default
  }
  switch (definition.type) {
    case 'boolean':
      if (typeof value === 'boolean') return value
      return String(value).toLowerCase() === 'true'
    case 'number': {
      const number = Number(value)
      return Number.isFinite(number) ? number : definition.default
    }
    default:
      return String(value)
  }
}

/**
 * NAME=value lines, for settings that have no field of their own yet.
 *
 * Anything that is not a line of that shape is skipped rather than guessed at:
 * a typo should lose one variable, not the rest of the block.
 */
function parseExtraEnvironment(text) {
  const extra = {}
  for (const line of String(text || '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const at = trimmed.indexOf('=')
    if (at <= 0) continue
    const name = trimmed.slice(0, at).trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) continue
    extra[name] = trimmed.slice(at + 1).trim()
  }
  return extra
}

/**
 * Every environment variable these settings amount to.
 *
 * A variable is absent from the result when it should not be set at all:
 * `unset` and `false` are the same thing to most of what reads these, and
 * several of them treat the presence of the variable as the answer.
 *
 * @param {Record<string, any>} values  the stored settings
 * @return {Record<string, string>}
 */
export function resolveEnvironment(values) {
  const environment = {}

  for (const definition of SETTINGS) {
    if (!definition.env || definition.seedOnly) continue
    const value = coerce(definition, values[definition.key])
    if (value === true) {
      environment[definition.env] = 'true'
    } else if (value === false || value == null || value === '') {
      continue
    } else {
      environment[definition.env] = String(value)
    }
  }

  // A reference manager is reachable only if its kind of linked file is
  // allowed, so configuring one implies allowing it. Asking an administrator
  // to also remember a second list is how it ends up configured but invisible.
  const linked = new Set(
    (environment.ENABLED_LINKED_FILE_TYPES || '')
      .split(',')
      .map(entry => entry.trim())
      .filter(Boolean)
  )
  if (values.zoteroClientId) linked.add('zotero')
  if (values.mendeleyClientId) linked.add('mendeley')
  if (linked.size > 0) {
    environment.ENABLED_LINKED_FILE_TYPES = Array.from(linked).join(',')
  }

  // Last, so that a name set here wins: this is the escape hatch, and it is
  // no use if a field it does not know about overrides it.
  Object.assign(environment, parseExtraEnvironment(values.extraEnvironment))

  return environment
}

/** The variables these settings could set, whether or not they are set now. */
export function managedNames(values) {
  const names = new Set()
  for (const definition of SETTINGS) {
    if (definition.env && !definition.seedOnly) names.add(definition.env)
  }
  names.add('ENABLED_LINKED_FILE_TYPES')
  for (const name of Object.keys(parseExtraEnvironment(values?.extraEnvironment))) {
    names.add(name)
  }
  return names
}

export default { resolveEnvironment, managedNames }
