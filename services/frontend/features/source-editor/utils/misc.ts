import type { EditorState } from '@codemirror/state'
import type { SearchQuery } from '@codemirror/search'
import type { Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'

/** The RegExp a search query describes, or a throw when it is not one. */
export const createRegExp = (searchQuery: SearchQuery) => {
  const flags = 'gmu' + (searchQuery.caseSensitive ? '' : 'i')
  return new RegExp(searchQuery.search, flags)
}

export const isInvalidRegExp = (searchQuery: SearchQuery): boolean => {
  try {
    createRegExp(searchQuery)
    return false
  } catch {
    return true
  }
}

/**
 * Whether the cursor is on an empty line with no selection, which is when
 * there is nothing to anchor a comment to.
 */
export function isCursorOnEmptyLine(state: EditorState): boolean {
  const { main } = state.selection
  if (!main.empty) {
    return false
  }
  const line = state.doc.lineAt(main.head)
  return line.text.trim().length === 0
}

export const getFileExtension = (filename: string): string | null => {
  const parts = filename.split('.')
  if (parts.length < 2) {
    return null
  }
  return parts[parts.length - 1]!.toLowerCase()
}

const IMAGE_FILE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'pdf', 'eps', 'svg']

export const hasImageExtension = (filename: string) => {
  const extension = getFileExtension(filename)
  return extension !== null && IMAGE_FILE_EXTENSIONS.includes(extension)
}

/** The file kinds the source editor treats as TeX. */
export function isValidTeXFile(filename: string) {
  return /\.(tex|txt|ltx|rtex|rnw|latex|bib|md|rmd)$/i.test(filename)
}

/** Whether the visual editor can show this file. */
export function isVisualEditorAvailable(filename: string): boolean {
  return isValidTeXFile(filename) && !/\.bib$/i.test(filename)
}

/**
 * Remembers a theme by its content, so asking for the same one twice does
 * not build a second extension -- which would reconfigure the editor for no
 * change.
 */
export class ThemeCache {
  private cache: Map<string, Extension> = new Map()

  public get: typeof EditorView.theme = (styleMod, options) => {
    const key = JSON.stringify({ styleMod, options })
    const existing = this.cache.get(key)
    if (existing) {
      return existing
    }
    const theme = EditorView.theme(styleMod, options)
    this.cache.set(key, theme)
    return theme
  }
}
