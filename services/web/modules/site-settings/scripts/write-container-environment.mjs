// Puts the settings an administrator changed into the container environment,
// before any service starts.
//
// Most of what an operator used to configure is read while a settings file is
// being evaluated -- the upload limit that nginx is generated from, the S3
// buckets filestore and history-v1 connect to, which external authentication
// exists. By the time the web process could read the database, all of that has
// already happened, in this process and in every other one. So the settings are
// materialised here instead, into the directory the container builds every
// service's environment from.
//
// This is why those settings say they need a restart: this script is the
// moment they take effect.
//
// Nothing here is fatal. A site whose database is not up yet should start with
// the environment it already has and fail in the check that exists for that,
// with the message that check has, rather than here.

import fs from 'node:fs/promises'
import Path from 'node:path'

const ENVIRONMENT_DIR =
  process.env.CONTAINER_ENVIRONMENT_DIR || '/etc/container_environment'
// Not inside the environment directory: everything in there becomes a
// variable, and a manifest is not one. On the data volume so that it survives
// the container being replaced, which is when it matters.
const MANIFEST =
  process.env.SITE_SETTINGS_MANIFEST ||
  '/var/lib/overleaf/data/site-settings-env.json'
const DOCUMENT_ID = 'site'

async function readManifest() {
  try {
    const raw = await fs.readFile(MANIFEST, 'utf8')
    return new Set(JSON.parse(raw))
  } catch {
    // Nothing written yet, which is the normal first start.
    return new Set()
  }
}

async function main() {
  const { db, connectionPromise } = await import(
    '../../../app/src/infrastructure/mongodb.mjs'
  )
  const { resolveEnvironment } = await import(
    '../app/src/SiteSettingsEnvironment.mjs'
  )

  await connectionPromise
  const stored = await db.siteSettings.findOne({ _id: DOCUMENT_ID })
  if (!stored) {
    // A site that has never been set up. The environment it was started with
    // is all there is, and the web process seeds the settings from it.
    console.log('No stored site settings yet, leaving the environment alone')
    return
  }

  const environment = resolveEnvironment(stored.values || {})
  const written = await readManifest()

  // Variables this wrote last time and does not write now have been turned
  // off. Only those are removed: a variable that was set in the compose file
  // and never here is not ours to take away.
  for (const name of written) {
    if (!(name in environment)) {
      await fs.rm(Path.join(ENVIRONMENT_DIR, name), { force: true })
    }
  }

  for (const [name, value] of Object.entries(environment)) {
    await fs.writeFile(Path.join(ENVIRONMENT_DIR, name), value, { mode: 0o600 })
  }

  await fs.writeFile(MANIFEST, JSON.stringify(Object.keys(environment)), {
    mode: 0o600,
  })
  console.log(
    `Applied ${Object.keys(environment).length} settings from the database`
  )
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(
      'Could not read the site settings; starting with the environment as it is'
    )
    console.error(error.message)
    process.exit(0)
  })
