/**
 * Puts the spell checker's assets where the browser can fetch them.
 *
 * Two things, for the same reason as pdf.js and MathJax: neither is imported
 * by the bundle. The WebAssembly module is fetched at run time by Emscripten's
 * glue code, and the dictionaries are fetched by the worker for whichever
 * language the person chose -- so both have to be files at URLs.
 *
 * All the dictionaries ship, as they do in the original. It is a few hundred
 * megabytes in the image and nothing at all over the wire: a browser fetches
 * the one language it is set to and no other.
 */

import { copyFileSync, cpSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const publicDir = join(here, '..', 'public')

const wasm = join(here, '..', 'features', 'source-editor', 'hunspell', 'wasm', 'hunspell.wasm')
const wasmTarget = join(publicDir, 'hunspell', 'hunspell.wasm')
mkdirSync(dirname(wasmTarget), { recursive: true })
copyFileSync(wasm, wasmTarget)
console.log('hunspell wasm copied to public/hunspell/hunspell.wasm')

const wordlists = join(here, '..', 'features', 'source-editor', 'hunspell', 'wordlists')
const wordlistTarget = join(publicDir, 'hunspell', 'wordlists')
mkdirSync(wordlistTarget, { recursive: true })
for (const list of ['en_US.txt']) {
  copyFileSync(join(wordlists, list), join(wordlistTarget, list))
}
console.log('hunspell word lists copied to public/hunspell/wordlists')

const dictionaries = join(here, '..', 'node_modules', '@overleaf', 'dictionaries', 'dictionaries')
if (existsSync(dictionaries)) {
  const target = join(publicDir, 'dictionaries')
  mkdirSync(dirname(target), { recursive: true })
  cpSync(dictionaries, target, { recursive: true })
  console.log('dictionaries copied to public/dictionaries')
} else {
  // Not fatal: the editor works without a spell checker, and says so by
  // simply not underlining anything.
  console.log('dictionaries: @overleaf/dictionaries is not installed, skipped')
}
