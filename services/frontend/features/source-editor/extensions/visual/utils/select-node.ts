import { EditorSelection } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { SyntaxNode } from '@lezer/common'

/**
 * Selects what is inside a node's braces.
 *
 * From node.from + 1 to node.to - 1, so the braces themselves are left out:
 * this is used to put the cursor into an argument somebody is about to
 * replace, and selecting the braces would have them type over the syntax.
 */
export const selectNode = (view: EditorView, node: SyntaxNode) => {
  view.dispatch({
    selection: EditorSelection.single(node.from + 1, node.to - 1),
    scrollIntoView: true,
  })
}
