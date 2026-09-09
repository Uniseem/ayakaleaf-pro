import { Annotation } from '@codemirror/state'

/**
 * Marks a transaction made to open the editor's context menu, so listeners
 * that react to selection changes (autocomplete, for one) leave it alone.
 * The menu itself belongs to the review phase.
 */
export const openContextMenuAnnotation = Annotation.define<boolean>()
