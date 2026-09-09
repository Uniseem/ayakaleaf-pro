import { sourceOnly } from './visual/visual'
import { highlightSpecialChars as _highlightSpecialChars } from '@codemirror/view'

// non standard space characters (https://jkorpela.fi/chars/spaces.html)
const nonStandardSpaces = String.fromCodePoint(
  0x00a0,
  0x1680,
  0x180e,
  0x2000,
  0x2001,
  0x2002,
  0x2003,
  0x2004,
  0x2005,
  0x2006,
  0x2007,
  0x2008,
  0x2009,
  0x200a,
  0x200b,
  0x202f,
  0x205f,
  0x3000,
  0xfeff
)

/**
 * The built-in extension which highlights unusual whitespace characters,
 * configured to highlight additional space characters.
 */
export const highlightSpecialChars = (visual: boolean) =>
  sourceOnly(
    visual,
    _highlightSpecialChars({
      addSpecialChars: new RegExp(`[${nonStandardSpaces}]`, /x/.unicode != null ? 'gu' : 'g'),
    })
  )
