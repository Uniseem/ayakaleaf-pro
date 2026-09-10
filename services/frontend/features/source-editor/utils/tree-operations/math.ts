import { getEnvironmentName } from './environments'
import { EditorState } from '@codemirror/state'
import { SyntaxNode, SyntaxNodeRef } from '@lezer/common'
import { ancestorNodeOfType } from './ancestors'

/**
 * A piece of maths, and how it should be typeset.
 *
 * Which text to hand to the typesetter is not the same question as where the
 * maths starts: an environment has to be passed whole, because the environment
 * name is what says whether it is aligned, but the position that identifies it
 * is where its content begins.
 */
export type MathContainer = {
  content: string
  displayMode: boolean
  passToMathJax: boolean
  pos: number
}

export const mathAncestorNode = (state: EditorState, pos: number) =>
  ancestorNodeOfType(state, pos, '$MathContainer') ||
  ancestorNodeOfType(state, pos, 'EquationEnvironment') ||
  // EquationArrayEnvironment can be nested inside EquationEnvironment.
  ancestorNodeOfType(state, pos, 'EquationArrayEnvironment')

export const parseMathContainer = (
  state: EditorState,
  nodeRef: SyntaxNodeRef,
  ancestorNode: SyntaxNode
): MathContainer | null => {
  // The content of the Math element, without braces.
  const innerContent = state.doc.sliceString(nodeRef.from, nodeRef.to).trim()

  if (!innerContent.length) {
    return null
  }

  let content = innerContent
  let displayMode = false
  let passToMathJax = true
  let pos = nodeRef.from

  if (ancestorNode.type.is('$Environment')) {
    const environmentName = getEnvironmentName(ancestorNode, state)
    if (environmentName) {
      // The outer content of the environments MathJax supports, because the
      // environment is part of what it is being asked to typeset.
      if (environmentName === 'tikzcd') {
        passToMathJax = false
      }
      if (environmentName !== 'math' && environmentName !== 'displaymath') {
        content = state.doc.sliceString(ancestorNode.from, ancestorNode.to).trim()
        pos = ancestorNode.from
      }

      if (environmentName !== 'math') {
        displayMode = true
      }
    }
  } else {
    if (ancestorNode.type.is('BracketMath') || Boolean(ancestorNode.getChild('DisplayMath'))) {
      displayMode = true
    }
  }

  return { content, displayMode, passToMathJax, pos }
}
