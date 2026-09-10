import { ensureSyntaxTree } from '@codemirror/language'
import type { EditorState } from '@codemirror/state'
import type { SyntaxNode, SyntaxNodeRef } from '@lezer/common'
import { previousSiblingIs } from './common'
import { NodeIntersectsChangeFn, ProjectionItem } from './projection'
import { FigureData } from '../../extensions/figure-modal'

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

/**
 * Reads one `figure` environment into the positions the figure modal edits.
 *
 * Returns null rather than a partial result whenever the environment holds
 * more than one caption, label or graphic: the modal rewrites by position, and
 * with two candidates there is no way to know which one the user meant. The
 * source stays visible in that case, which is the honest outcome.
 */
export function parseFigureData(
  figureEnvironmentNode: SyntaxNode,
  state: EditorState
): FigureData | null {
  let caption: FigureData['caption'] = null
  let label: FigureData['label'] = null
  let file: FigureData['file'] | undefined
  let width: FigureData['width']
  let unknownGraphicsArguments: FigureData['unknownGraphicsArguments']
  let graphicsCommand: FigureData['graphicsCommand'] | undefined
  let graphicsCommandArguments: FigureData['graphicsCommandArguments'] = null

  const from = figureEnvironmentNode.from
  const to = figureEnvironmentNode.to

  let error = false
  figureEnvironmentNode.cursor().iterate((node: SyntaxNodeRef) => {
    if (error) {
      return false
    }
    if (node.type.is('Caption')) {
      if (caption) {
        // Multiple captions
        error = true
        return false
      }
      caption = {
        from: node.from,
        to: node.to,
      }
    }
    if (node.type.is('Label')) {
      if (label) {
        // Multiple labels
        error = true
        return false
      }
      label = {
        from: node.from,
        to: node.to,
      }
    }
    if (node.type.is('IncludeGraphics') || node.type.is('IncludeSvg')) {
      const isIncludeSvg = node.type.is('IncludeSvg')
      if (file) {
        // Multiple figure
        error = true
        return false
      }
      graphicsCommand = {
        from: node.from,
        to: node.to,
      }
      const argumentNodeName = isIncludeSvg
        ? 'IncludeSvgArgument'
        : 'IncludeGraphicsArgument'
      const content = node.node
        .getChild(argumentNodeName)
        ?.getChild('FilePathArgument')
        ?.getChild('LiteralArgContent')
      if (!content) {
        error = true
        return false
      }
      // \includesvg stores path without .svg extension, but we add it for consistency
      file = {
        from: content.from,
        to: content.to,
        path: state.sliceDoc(content.from, content.to) + (isIncludeSvg ? '.svg' : ''),
      }
      const optionalArgs = node.node
        .getChild('OptionalArgument')
        ?.getChild('ShortOptionalArg')
      if (!optionalArgs) {
        width = undefined
        return false
      }
      graphicsCommandArguments = {
        from: optionalArgs.from,
        to: optionalArgs.to,
      }
      const optionalArgContent = state.sliceDoc(optionalArgs.from, optionalArgs.to)
      const widthMatch = optionalArgContent.match(
        /^width=([0-9]|(?:[0-9]*\.[0-9]+)|(?:[0-9]+\.))\\(linewidth|pagewidth|textwidth|hsize|columnwidth)$/
      )
      if (widthMatch) {
        width = parseFloat(widthMatch[1]!)
        if (widthMatch[2] !== 'linewidth') {
          // We shouldn't edit any width other that linewidth
          unknownGraphicsArguments = optionalArgContent
        }
      } else {
        unknownGraphicsArguments = optionalArgContent
      }
    }
  })
  if (error) {
    return null
  }
  if (graphicsCommand === undefined || file === undefined) {
    return null
  }
  return new FigureData({
    caption,
    label,
    file,
    from,
    to,
    width,
    unknownGraphicsArguments,
    graphicsCommand,
    graphicsCommandArguments,
  })
}
