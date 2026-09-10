import { Line } from '@codemirror/state'
import { SyntaxNodeRef } from '@lezer/common'

/**
 * Whether a node is the only thing on its line.
 *
 * A command alone on a line is drawn as a block; the same command with text
 * beside it has to stay inline, or the paragraph it is part of comes apart.
 */
export const lineContainsOnlyNode = (line: Line, nodeRef: SyntaxNodeRef) =>
  line.text.trim().length === nodeRef.to - nodeRef.from
