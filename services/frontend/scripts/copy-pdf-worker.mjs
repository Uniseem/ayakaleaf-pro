/**
 * Puts pdf.js's worker where the browser can fetch it.
 *
 * The viewer parses PDFs in a worker, and the worker has to be a file at a
 * URL -- it is started by the browser, not imported by the bundle, so no
 * amount of bundler configuration produces it. Copying it from the installed
 * package at build time is the version that cannot drift: the file that ships
 * is always the one belonging to the pdfjs-dist that was installed, and there
 * is no vendored megabyte in the repository to forget to update.
 */

import { copyFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const from = join(here, '..', 'node_modules', 'pdfjs-dist', 'build', 'pdf.worker.min.mjs')
const into = join(here, '..', 'public', 'pdf.worker.min.mjs')

mkdirSync(dirname(into), { recursive: true })
copyFileSync(from, into)
console.log('pdf.js worker copied to public/pdf.worker.min.mjs')
