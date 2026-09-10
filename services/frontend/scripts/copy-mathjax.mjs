/**
 * Puts MathJax where the browser can fetch it.
 *
 * MathJax is loaded as a script at run time rather than imported, because it
 * fetches its own extensions, its font data and its accessibility rules by URL
 * relative to wherever the main file was served from. Bundling the entry point
 * would leave those fetches pointing nowhere, so the whole component tree is
 * copied out of the installed package instead -- the same four sets of files
 * the original's webpack copy step takes.
 *
 * The directory is named after the installed version, so a version bump lands
 * at a new URL and no browser can serve half of one release out of its cache
 * alongside half of another.
 */

import { cpSync, existsSync, mkdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const pkg = join(here, '..', 'node_modules', 'mathjax')
const { version } = JSON.parse(readFileSync(join(pkg, 'package.json'), 'utf8'))
const target = join(here, '..', 'public', 'mathjax', version)

// tex-svg.js is the entry point; the rest is what it loads on demand.
// https://www.npmjs.com/package/mathjax#hosting-your-own-copy-of-the-mathjax-components
const contents = [
  'tex-svg.js',
  join('input', 'tex', 'extensions'),
  'ui',
  'sre',
]

mkdirSync(target, { recursive: true })

for (const item of contents) {
  const source = join(pkg, item)
  if (!existsSync(source)) {
    console.log(`mathjax: ${item} not in this release, skipped`)
    continue
  }
  const destination = join(target, item)
  mkdirSync(dirname(destination), { recursive: true })
  cpSync(source, destination, { recursive: true })
}

console.log(`mathjax ${version} copied to public/mathjax/${version}`)
