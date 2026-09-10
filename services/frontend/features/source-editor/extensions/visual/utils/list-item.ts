import { EditorState } from '@codemirror/state'
import { IndentContext, indentString } from '@codemirror/language'

/**
 * The text of a new `\item`, indented to match where it is going.
 *
 * Taken from the line rather than from a setting: a list inside a list is
 * indented further than its parent, and a new item that ignores that lands
 * against the margin in a document where nothing else does.
 */
export const createListItem = (state: EditorState, pos: number) => {
  const cx = new IndentContext(state)
  const columns = cx.lineIndent(pos)
  const indent = indentString(state, columns)
  return `${indent}\\item `
}
