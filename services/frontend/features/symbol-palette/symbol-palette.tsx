'use client'

import { SymbolPaletteContent } from './components/symbol-palette-content'
import type { Symbol } from './utils/categories'

/**
 * The pane under the editor.
 *
 * A chosen symbol is announced on the window rather than inserted from here:
 * the extension that owns the CodeMirror view is what puts text in the
 * document, and it is listening.
 */
export default function SymbolPalette() {
  const handleSelect = (symbol: Symbol) => {
    window.dispatchEvent(new CustomEvent('editor:insert-symbol', { detail: symbol }))
  }
  return <SymbolPaletteContent handleSelect={handleSelect} />
}
