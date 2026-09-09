import * as PDFJS from 'pdfjs-dist'
import type { DocumentInitParameters } from 'pdfjs-dist/types/src/display/api'

export { PDFJS }

// The worker is a file the browser fetches, copied out of the package at
// build time by scripts/copy-pdf-worker.mjs, as are the resources below.
if (typeof window !== 'undefined') {
  PDFJS.GlobalWorkerOptions.workerSrc = '/pdf.worker.min.mjs'
}

export const imageResourcesPath = '/pdfjs/images/'
const cMapUrl = '/pdfjs/cmaps/'
const wasmUrl = '/pdfjs/wasm/'
const iccUrl = '/pdfjs/iccs/'
const standardFontDataUrl = '/pdfjs/standard_fonts/'

const disableFontFace =
  typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('disable-font-face') === 'true'

export const loadPdfDocumentFromUrl = (url: string, options: Partial<DocumentInitParameters> = {}) =>
  PDFJS.getDocument({
    url,
    cMapUrl,
    wasmUrl,
    iccUrl,
    standardFontDataUrl,
    disableFontFace,
    disableAutoFetch: true, // only fetch the data needed for the displayed pages
    disableStream: true,
    isEvalSupported: false,
    enableXfa: false,
    withCredentials: true,
    ...options,
  })
