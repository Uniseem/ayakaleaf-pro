import { ensureSyntaxTree } from '@codemirror/language'
import type { EditorState } from '@codemirror/state'
import type { SyntaxNode, SyntaxNodeRef } from '@lezer/common'
import { previousSiblingIs } from './common'
import { NodeIntersectsChangeFn, ProjectionItem } from './projection'

const HUNDRED_MS = 100

export class Environment extends ProjectionItem {
  readonly title: string = ''
  readonly type: 'usage' | 'definition' = 'usage'
  readonly raw: string = ''
}

export const enterNode = (
  state: EditorState,
  node: SyntaxNodeRef,
  items: Environment[],
  nodeIntersectsChange: NodeIntersectsChangeFn
): boolean | void => {
  if (node.type.is('EnvNameGroup')) {
    if (!nodeIntersectsChange(node.node)) {
      return false
    }
    if (!node.node.prevSibling?.type.is('Begin')) {
      return false
    }
    const openBraceNode = node.node.getChild('OpenBrace')
    if (!openBraceNode) {
      return false
    }
    const envNameNode = openBraceNode.node.nextSibling
    if (!envNameNode) {
      return false
    }
    const envNameText = state.doc.sliceString(envNameNode.from, envNameNode.to)
    if (envNameText.length < 1) {
      return false
    }
    items.push({
      title: envNameText,
      from: envNameNode.from,
      to: envNameNode.to,
      line: state.doc.lineAt(envNameNode.from).number,
      toLine: state.doc.lineAt(envNameNode.to).number,
      type: 'usage',
      raw: state.sliceDoc(node.from, node.to),
    })
  } else if (node.type.is('NewEnvironment') || node.type.is('RenewEnvironment')) {
    if (!nodeIntersectsChange(node.node)) {
      return false
    }
    const envNameNode = node.node.getChild('LiteralArgContent')
    if (!envNameNode) {
      return
    }
    const envNameText = state.doc.sliceString(envNameNode.from, envNameNode.to)
    if (!envNameText) {
      return
    }
    items.push({
      title: envNameText,
      from: envNameNode.from,
      to: envNameNode.to,
      line: state.doc.lineAt(envNameNode.from).number,
      toLine: state.doc.lineAt(envNameNode.to).number,
      type: 'definition',
      raw: state.sliceDoc(node.from, node.to),
    })
  }
}

export const cursorIsAtBeginEnvironment = (state: EditorState, pos: number): boolean | undefined => {
  const tree = ensureSyntaxTree(state, pos, HUNDRED_MS)
  if (!tree) {
    return
  }
  let thisNode = tree.resolve(pos)
  if (!thisNode) {
    return
  }
  if (thisNode.type.is('EnvNameGroup') && previousSiblingIs(state, pos, 'Begin')) {
    return true
  } else if (thisNode.type.is('$Environment') || (thisNode.type.is('LaTeX') && pos === state.doc.length)) {
    thisNode = tree.resolve(pos, -1)
    if (!thisNode) {
      return
    }
    if (thisNode.type.is('OpenBrace') || thisNode.type.is('$EnvName')) {
      return true
    }
  }
}

export const cursorIsAtEndEnvironment = (state: EditorState, pos: number): boolean | undefined => {
  const tree = ensureSyntaxTree(state, pos, HUNDRED_MS)
  if (!tree) {
    return
  }
  let thisNode = tree.resolve(pos)
  if (!thisNode) {
    return
  }
  if (thisNode.type.is('EnvNameGroup') && previousSiblingIs(state, pos, 'End')) {
    return true
  } else if (thisNode.type.is('$Environment') || thisNode.type.is('Content')) {
    thisNode = tree.resolve(pos, -1)
    if (!thisNode) {
      return
    }
    if (thisNode.type.is('OpenBrace') || thisNode.type.is('EnvName')) {
      return true
    }
  }
}

/**
 * @param node A node of type `$Environment`, `BeginEnv`, or `EndEnv`
 * @returns The environment name or null if a name cannot be found
 */
export function getEnvironmentName(node: SyntaxNode | null, state: EditorState): string | null {
  if (node?.type.is('$Environment')) {
    node = node.getChild('BeginEnv')
  }
  if (!node?.type.is('BeginEnv') && !node?.type.is('EndEnv')) {
    return null
  }
  const nameNode = node?.getChild('EnvNameGroup')?.getChild('OpenBrace')?.nextSibling
  if (!nameNode) {
    return null
  }
  if (nameNode.type.is('CloseBrace')) {
    return null
  }
  return state.sliceDoc(nameNode.from, nameNode.to)
}

export const getUnstarredEnvironmentName = (node: SyntaxNode | null, state: EditorState): string | undefined =>
  getEnvironmentName(node, state)?.replace(/\*$/, '')

export function getEnvironmentArguments(environmentNode: SyntaxNode) {
  return environmentNode.getChild('BeginEnv')?.getChildren('TextArgument')
}
