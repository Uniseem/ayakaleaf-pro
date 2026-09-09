// @ts-check
/**
 * Builds lib/locales/en.json from the strings this client actually uses.
 *
 * The original keeps every string in services/web/locales/<lang>.json and
 * refers to them by key, so the same keys here mean the same words on screen.
 * Rather than ship the whole 3,000-key file, this scans the source for
 * t('key'), i18nKey="key" and the tKey() helper, and copies just those keys --
 * with their _plural forms -- into a file the client bundles.
 *
 * Run it after adding a key:  node scripts/extract-translations.mjs
 * A key that is not in the original's file is reported and left out, so a
 * typo shows up here rather than as a raw key on screen.
 */
import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..')
const source = JSON.parse(
  readFileSync(join(root, '..', 'web', 'locales', 'en.json'), 'utf8')
)

/** @param {string} dir @param {string[]} out */
function walk(dir, out) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === '.next' || name.startsWith('.')) {
      continue
    }
    const path = join(dir, name)
    if (statSync(path).isDirectory()) {
      walk(path, out)
    } else if (/\.(tsx?|mjs)$/.test(name) && !path.includes('scripts')) {
      out.push(path)
    }
  }
  return out
}

const files = walk(root, [])
const keys = new Set()
const patterns = [
  /\bt\(\s*'([a-zA-Z0-9_]+)'/g,
  /\bt\(\s*"([a-zA-Z0-9_]+)"/g,
  /i18nKey=["']([a-zA-Z0-9_]+)["']/g,
  /\btKey\(\s*'([a-zA-Z0-9_]+)'/g,
  /\/\*\s*i18n:\s*([a-zA-Z0-9_ ,]+)\s*\*\//g,
]
for (const file of files) {
  const text = readFileSync(file, 'utf8')
  for (const pattern of patterns) {
    for (const match of text.matchAll(pattern)) {
      for (const key of match[1].split(',')) {
        keys.add(key.trim())
      }
    }
  }
}

const out = {}
const missing = []
for (const key of [...keys].sort()) {
  if (!key) continue
  if (key in source) {
    out[key] = source[key]
    if (`${key}_plural` in source) {
      out[`${key}_plural`] = source[`${key}_plural`]
    }
  } else {
    missing.push(key)
  }
}

writeFileSync(
  join(root, 'lib', 'locales', 'en.json'),
  JSON.stringify(out, null, 2) + '\n'
)
console.log(`${Object.keys(out).length} strings written to lib/locales/en.json`)
if (missing.length) {
  console.log('Not in the original en.json (left out):')
  for (const key of missing) console.log('  ' + key)
}
