import Settings from '@overleaf/settings'
import logger from '@overleaf/logger'
import RedisWrapper from '@overleaf/redis-wrapper'
import { db } from '../../../../app/src/infrastructure/mongodb.mjs'
import {
  SETTINGS,
  SECTIONS,
  definitionFor,
  isSecret,
} from './SiteSettingsCatalogue.mjs'

// The settings an operator can change now live in the database rather than in
// the environment, and this is what puts them into effect.
//
// @overleaf/settings hands every module the same object, so writing to it here
// is seen by everything that reads Settings.x at the time it reads it. That is
// what makes a setting take effect without a restart -- and it is also why the
// few settings that are read once during startup are marked `restart` in the
// catalogue and say so in the page.
//
// There is more than one web process, so a change made in one has to reach the
// others: they are told over Redis and re-read the document. A process that
// misses the message is wrong only until the next one arrives, and the stored
// document stays the truth.

const DOCUMENT_ID = 'site'
const CHANGE_CHANNEL = 'site-settings:changed'

let loaded = false
let current = {}
let seededFromEnvironment = false
let subscriber = null
let publisher = null

/** Reads a dot path out of an object. */
function readPath(root, path) {
  return path.split('.').reduce((node, key) => (node == null ? node : node[key]), root)
}

/** Writes a dot path into an object, making the objects on the way. */
function writePath(root, path, value) {
  const keys = path.split('.')
  const last = keys.pop()
  let node = root
  for (const key of keys) {
    if (node[key] == null || typeof node[key] !== 'object') {
      node[key] = {}
    }
    node = node[key]
  }
  node[last] = value
}

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

/** The value this setting had in the environment, if it had one. */
function fromEnvironment(definition) {
  if (!definition.env) return undefined
  const raw = process.env[definition.env]
  if (raw == null || raw === '') return undefined
  return coerce(definition, raw)
}

/**
 * The stored document, or one built from the environment.
 *
 * Seeding from the environment is what keeps an existing deployment working
 * across the upgrade: whatever was in its compose file becomes the first
 * stored value, and after that the page is the only place it changes.
 */
async function loadDocument() {
  const stored = await db.siteSettings.findOne({ _id: DOCUMENT_ID })
  if (stored) {
    seededFromEnvironment = stored.seededFromEnvironment === true
    return stored.values || {}
  }

  const seeded = {}
  for (const definition of SETTINGS) {
    const value = fromEnvironment(definition)
    if (value !== undefined) {
      seeded[definition.key] = value
    }
  }
  await db.siteSettings.updateOne(
    { _id: DOCUMENT_ID },
    {
      $set: { values: seeded, seededFromEnvironment: true },
      $currentDate: { updatedAt: true },
    },
    { upsert: true }
  )
  seededFromEnvironment = Object.keys(seeded).length > 0
  logger.info(
    { count: Object.keys(seeded).length },
    'seeded site settings from the environment'
  )
  return seeded
}

/** Puts the values into the Settings object every module reads. */
function apply(values) {
  for (const definition of SETTINGS) {
    const value = coerce(definition, values[definition.key])
    if (value === undefined || value === '') {
      // An empty value means "not set": leave whatever the defaults put there
      // rather than writing an empty string over it, so that a blank SMTP host
      // reads as no email rather than as a host named "".
      const existing = readPath(Settings, definition.path)
      if (existing !== undefined && values[definition.key] === undefined) {
        continue
      }
    }
    writePath(Settings, definition.path, value)
  }

  // A few settings are read from more than one place; keep those in step here
  // rather than making every reader know about this module.
  Settings.siteUrl = (Settings.siteUrl || '').replace(/\/+$/, '')

  // The identity manager refuses to link a provider it has not been told
  // about, so the social providers an administrator has configured are
  // published here. A provider with no client id is left out, which is what
  // hides its button.
  Settings.oauthProviders = Settings.oauthProviders || {}
  for (const [id, name] of [
    ['google', 'Google'],
    ['github', 'GitHub'],
  ]) {
    const configured = Settings.siteSettings?.oauth?.[id]
    if (configured?.clientId && configured?.clientSecret) {
      Settings.oauthProviders[id] = {
        name,
        // What the account settings page needs to draw the linking widget.
        descriptionKey: 'login_with_service',
        descriptionOptions: { service: name, appName: Settings.appName },
        linkPath: `/auth/${id}`,
      }
    } else {
      delete Settings.oauthProviders[id]
    }
  }
  Settings.siteSettings.enabledOAuthProviders = Object.keys(
    Settings.oauthProviders
  ).filter(id => id === 'google' || id === 'github')
  if (Settings.email && !Settings.email.parameters?.host) {
    // Nodemailer treats a transport with no host as a configuration error, and
    // the rest of the app treats a missing email block as "email is off".
    delete Settings.email
  }

  // Nobody can confirm an address on a site with no mail server, so the
  // requirement follows whether one is configured rather than being a separate
  // switch that can be left contradicting it.
  Settings.emailConfirmationDisabled = !Settings.email
  current = values
}

async function reload() {
  const values = await loadDocument()
  apply(values)
  return values
}

/**
 * Listens for changes made by another web process.
 *
 * Redis is already a hard dependency of the session store, so this adds no new
 * one. A failure to subscribe is logged rather than fatal: the process is then
 * correct until somebody changes a setting elsewhere, which is a worse
 * position to be in than this one but not a reason to refuse to start.
 */
function watchForChanges() {
  try {
    publisher = RedisWrapper.createClient(Settings.redis.websessions || Settings.redis.web)
    subscriber = RedisWrapper.createClient(Settings.redis.websessions || Settings.redis.web)
    subscriber.subscribe(CHANGE_CHANNEL)
    subscriber.on('message', () => {
      reload().catch(err =>
        logger.err({ err }, 'could not reload the site settings')
      )
    })
  } catch (err) {
    logger.warn({ err }, 'site settings will not follow changes made elsewhere')
  }
}

const SiteSettingsManager = {
  /** Called once during startup, after Mongo is reachable. */
  async initialize() {
    await reload()
    watchForChanges()
    loaded = true
  },

  isLoaded() {
    return loaded
  },

  /** The current value of one setting. */
  get(key) {
    const definition = definitionFor(key)
    if (!definition) return undefined
    return coerce(definition, current[key])
  },

  /**
   * Everything the admin page needs to draw itself.
   *
   * A secret is reported as set or not set rather than by value: an
   * administrator who can read the page is not necessarily somebody who should
   * be handed the SMTP password back.
   */
  describe() {
    return {
      sections: SECTIONS,
      // True on a deployment that was upgraded into this: its first values
      // came out of a compose file, and the page should say so, because the
      // compose file is not read again after that.
      seededFromEnvironment,
      settings: SETTINGS.map(definition => ({
        key: definition.key,
        section: definition.section,
        type: definition.type,
        label: definition.label,
        help: definition.help,
        options: definition.options,
        restart: definition.restart === true,
        // The variable this used to be read from, for somebody who knows the
        // site by its compose file.
        env: definition.env,
        value: isSecret(definition)
          ? undefined
          : coerce(definition, current[definition.key]),
        isSet: isSecret(definition)
          ? Boolean(current[definition.key])
          : undefined,
      })),
    }
  },

  /**
   * Stores a change and puts it into effect.
   *
   * A secret sent back as an empty string means "leave it alone", because that
   * is what the page shows for one that is already set; clearing one is done
   * by sending the string "-", which is not a password anybody meant to use.
   */
  async save(changes, adminUserId) {
    const values = { ...current }
    const applied = []

    for (const [key, raw] of Object.entries(changes)) {
      const definition = definitionFor(key)
      if (!definition) continue

      if (isSecret(definition)) {
        if (raw === '' || raw == null) continue
        values[key] = raw === '-' ? '' : String(raw)
      } else {
        values[key] = coerce(definition, raw)
      }
      applied.push(key)
    }

    await db.siteSettings.updateOne(
      { _id: DOCUMENT_ID },
      {
        $set: { values, updatedBy: adminUserId },
        $currentDate: { updatedAt: true },
      },
      { upsert: true }
    )
    apply(values)

    if (publisher) {
      publisher.publish(CHANGE_CHANNEL, String(Date.now()))
    }
    logger.info({ keys: applied, adminUserId }, 'site settings changed')
    return applied
  },

  /** For the tests, which need to put it back the way they found it. */
  async _reload() {
    return await reload()
  },
}

export default SiteSettingsManager
