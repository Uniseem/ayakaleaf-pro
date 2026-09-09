import { syntaxTree } from '@codemirror/language'
import type { Line } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import type { SyntaxNodeRef } from '@lezer/common'
import { noSpellCheckProp } from '../node-props'

/** A convenient wrapper around 'Normal' tokens. */
export class NormalTextSpan {
  public from: number
  public to: number
  public lineNumber: number
  public text: string
  public node: SyntaxNodeRef

  constructor(options: { from: number; to: number; lineNumber: number; text: string; node: SyntaxNodeRef }) {
    const { from, to, lineNumber, text, node } = options
    this.from = from
    this.to = to
    this.text = text
    this.node = node
    this.lineNumber = lineNumber
  }
}

export const getNormalTextSpansFromLine = (view: EditorView, line: Line): Array<NormalTextSpan> => {
  const lineNumber = line.number
  const tree = syntaxTree(view.state)
  const normalTextSpans: Array<NormalTextSpan> = []
  tree?.iterate({
    from: line.from,
    to: line.to,
    enter: (node: SyntaxNodeRef) => {
      if (node.type.prop(noSpellCheckProp)?.some(context => node.matchContext(context))) {
        return false
      }
      if (node.type.name === 'Normal') {
        normalTextSpans.push(
          new NormalTextSpan({
            from: node.from,
            to: node.to,
            text: view.state.doc.sliceString(node.from, node.to),
            lineNumber,
            node,
          })
        )
        return false
      }
      return true
    },
  })
  return normalTextSpans
}
