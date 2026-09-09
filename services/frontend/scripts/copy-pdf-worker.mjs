/**
 * Puts pdf.js's worker and resources where the browser can fetch them.
 *
 * The viewer parses PDFs in a worker, and the worker has to be a file at a
 * URL -- it is started by the browser, not imported by the bundle, so no
 * amount of bundler configuration produces it. The same goes for what the
 * viewer fetches while drawing: the character maps, the standard fonts, the
 * decoders and the annotation icons. Copying them from the installed package
 * at build time is the version that cannot drift: the files that ship are
 * always the ones belonging to the pdfjs-dist that was installed, and there
 * is no vendored megabyte in the repository to forget to update.
 */

import { copyFileSync, cpSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const pkg = join(here, '..', 'node_modules', 'pdfjs-dist')
const publicDir = join(here, '..', 'public')

const worker = join(publicDir, 'pdf.worker.min.mjs')
mkdirSync(dirname(worker), { recursive: true })
copyFileSync(join(pkg, 'build', 'pdf.worker.min.mjs'), worker)
console.log('pdf.js worker copied to public/pdf.worker.min.mjs')

// the resources, at the paths features/pdf-preview/util/pdf-js.ts names
const resources = [
  ['cmaps', 'cmaps'],
  ['standard_fonts', 'standard_fonts'],
  ['wasm', 'wasm'],
  ['iccs', 'iccs'],
  [join('web', 'images'), 'images'],
]

for (const [from, to] of resources) {
  const source = join(pkg, from)
  if (!existsSync(source)) {
    console.log(`pdf.js resources: ${from} not in this pdfjs-dist, skipped`)
    continue
  }
  const target = join(publicDir, 'pdfjs', to)
  mkdirSync(dirname(target), { recursive: true })
  cpSync(source, target, { recursive: true })
  console.log(`pdf.js resources copied to public/pdfjs/${to}`)
}
