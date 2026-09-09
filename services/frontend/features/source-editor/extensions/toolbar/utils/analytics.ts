import type { EditorView } from '@codemirror/view'
import { isVisual } from '../../visual/visual'

/**
 * The original reported toolbar and shortcut use to its analytics service.
 * There is no such service here; the events are surfaced on the window so
 * anything interested can listen, and otherwise go nowhere.
 */
export function emitCommandEvent(
  view: EditorView,
  key: string,
  command: string,
  segmentation?: Record<string, string | number | boolean>
) {
  const mode = isVisual(view) ? 'visual' : 'source'
  window.dispatchEvent(new CustomEvent('editor:analytics', { detail: { key, command, mode, ...segmentation } }))
}

export function emitToolbarEvent(view: EditorView, command: string) {
  emitCommandEvent(view, 'codemirror-toolbar-event', command)
}

export function emitShortcutEvent(view: EditorView, command: string, segmentation?: Record<string, string | number | boolean>) {
  emitCommandEvent(view, 'codemirror-shortcut-event', command, segmentation)
}
