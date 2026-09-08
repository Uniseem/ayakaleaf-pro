// Asks whether the catalogue's defaults describe the deployment or change it.
//
// Every setting declares a default. That default is meant to be what this
// deployment already does when nobody has set anything -- because a fresh
// install has no stored values, so the defaults are what get written into the
// environment and into Settings. A default that differs from the code's own is
// not a description, it is a change, applied to every deployment that adopts
// it and to nothing an operator asked for. Two got through by hand: the
// history backend, which moved history off the filesystem, and the session
// length, which was in seconds where the setting is milliseconds and signed
// everybody out after seven minutes.
//
// So: load the settings twice in fresh processes -- once with the environment
// as it is, once with the catalogue's defaults added to it -- and compare.
// Anything that differs is a default that does not describe what is there.
//
//     node modules/site-settings/scripts/check-defaults-change-nothing.mjs

import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { resolveEnvironment } from '../app/src/SiteSettingsEnvironment.mjs'

const run = promisify(execFile)

const DUMP = `
import Settings from '@overleaf/settings'
process.stdout.write(JSON.stringify(Settings, (key, value) =>
  typeof value === 'function' ? '[function]' : value
))
`

async function settingsWith(environment) {
  const { stdout } = await run(
    process.execPath,
    ['--input-type=module', '--eval', DUMP],
    { env: environment, maxBuffer: 32 * 1024 * 1024, cwd: process.cwd() }
  )
  return JSON.parse(stdout)
}

/** Every leaf, as a path -> value map, so two shapes can be compared. */
function flatten(node, prefix = '', out = {}) {
  if (node === null || typeof node !== 'object') {
    out[prefix] = node
    return out
  }
  for (const [key, value] of Object.entries(node)) {
    flatten(value, prefix ? `${prefix}.${key}` : key, out)
  }
  return out
}

const baseline = await settingsWith(process.env)
const withDefaults = await settingsWith({
  ...process.env,
  ...resolveEnvironment({}),
})

const before = flatten(baseline)
const after = flatten(withDefaults)

const changed = []
for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
  const a = JSON.stringify(before[key])
  const b = JSON.stringify(after[key])
  if (a !== b) changed.push({ key, before: a, after: b })
}

if (changed.length === 0) {
  console.log('The catalogue defaults change nothing about this deployment.')
  process.exit(0)
}

console.log(`${changed.length} setting(s) would be changed by the defaults:\n`)
for (const { key, before: a, after: b } of changed) {
  console.log(`  ${key}`)
  console.log(`    is      ${a}`)
  console.log(`    becomes ${b}`)
}
console.log(
  '\nEach of these is a default that does not match what the code does with' +
    '\nthe variable unset. Fix the default, not the code.'
)
process.exit(1)
