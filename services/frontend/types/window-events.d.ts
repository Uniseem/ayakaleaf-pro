/**
 * The custom events the app sends on `window`.
 *
 * Declared here so that a listener for one is checked like any other: the
 * event name has to be one that is really dispatched, and the handler's
 * argument carries whatever detail that event was defined with. An event
 * added without a line here is a type error at the listener, which is the
 * point -- a typo in an event name is otherwise silent forever.
 */

import type { PastedImageData } from '@/features/source-editor/utils/paste-image'

declare global {
  interface WindowEventMap {
    'add-new-review-comment': CustomEvent<unknown>
    'cm:emacs-close-search-panel': CustomEvent<unknown>
    'cursor:editor:update': CustomEvent<unknown>
    'editor:analytics': CustomEvent<Record<string, unknown>>
    'editor:focus': Event
    'editor:full-project-search': CustomEvent<unknown>
    'editor:geometry-change': CustomEvent<unknown>
    'editor:insert-symbol': CustomEvent<{ command: string }>
    'editor:lint': CustomEvent<unknown>
    'editor:metadata-outdated': CustomEvent<unknown>
    'editor:scroll-position-restored': Event
    'editor:selection': CustomEvent<unknown>
    'editor:visual-switch': Event
    'figure-modal:open': CustomEvent<{
      source: number
      fileId?: string
      filePath?: string
    }>
    'figure-modal:open-modal': CustomEvent<unknown>
    'figure-modal:paste-image': CustomEvent<PastedImageData>
    'ide:goto-line': CustomEvent<unknown>
    'ide:reject-changes': CustomEvent<unknown>
    'ide:show-in-pdf': CustomEvent<unknown>
    'ide:show-toast': CustomEvent<unknown>
    'pdf:recompile': Event
    'scroll:editor:update': CustomEvent<unknown>
    'search-panel-before-doc-change': CustomEvent<unknown>
    'synctex:sync-to-position': CustomEvent<unknown>
    'toggle-track-changes': Event
    'ui:open-rail-modal': CustomEvent<unknown>
  }
}

export {}
