import { EditorState } from '@codemirror/state'
import { SyntaxNode } from '@lezer/common'
import { COMMAND_SUBSTITUTIONS } from '../visual-widgets/character'

type Markup = {
  elementType: keyof HTMLElementTagNameMap
  className?: string
}

/**
 * The formatting commands that have an HTML equivalent.
 *
 * Only these. A command whose effect cannot be shown is left as source, on the
 * principle that hiding markup without conveying its meaning loses
 * information rather than saving space.
 */
const textFormattingMarkupMap = new Map<string, Markup>([
  ['TextBoldCommand', { elementType: 'b' }],
  ['TextItalicCommand', { elementType: 'i' }],
  ['TextSmallCapsCommand', { elementType: 'span', className: 'ol-cm-command-textsc' }],
  ['TextTeletypeCommand', { elementType: 'span', className: 'ol-cm-command-texttt' }],
  ['TextSuperscriptCommand', { elementType: 'sup' }],
  ['TextSubscriptCommand', { elementType: 'sub' }],
  ['EmphasisCommand', { elementType: 'em' }],
  ['UnderlineCommand', { elementType: 'span', className: 'ol-cm-command-underline' }],
])

const markupMap = new Map<string, Markup>([
  ['\\and', { elementType: 'span', className: 'ol-cm-command-and' }],
])

/**
 * Typesets a little LaTeX into a DOM element.
 *
 * Not maths: this walks the syntax tree the editor already has and turns the
 * handful of formatting commands above into elements. Anything mathematical
 * has to go through MathJax afterwards, which is the caller's business because
 * it is asynchronous and this is not.
 */
export function typesetNodeIntoElement(
  node: SyntaxNode,
  element: HTMLElement,
  getText: EditorState | ((from: number, to: number) => string)
) {
  if (getText instanceof EditorState) {
    getText = getText.sliceDoc.bind(getText)
  }
  const readText = getText

  // A TextArgument's braces are syntax, not content.
  const argument = node.getChild('LongArg')
  if (argument) {
    node = argument
  }

  const ancestorStack = [element]

  const ancestor = () => ancestorStack[ancestorStack.length - 1]!
  const popAncestor = () => ancestorStack.pop()!
  const pushAncestor = (element: HTMLElement) => ancestorStack.push(element)

  let from = node.from

  const addMarkup = (markup: Markup, childNode: SyntaxNode) => {
    const element = document.createElement(markup.elementType) as HTMLElement
    if (markup.className) {
      element.classList.add(markup.className)
    }
    pushAncestor(element)
    from = chooseFrom(childNode)
  }

  node.cursor().iterate(
    function enter(childNodeRef) {
      const childNode = childNodeRef.node

      if (from < childNode.from) {
        ancestor().append(document.createTextNode(readText(from, childNode.from)))
        from = childNode.from
      }

      // Commands the grammar knows about.
      const markup = textFormattingMarkupMap.get(childNode.type.name)
      if (markup) {
        addMarkup(markup, childNode)
        return
      }

      // And the ones it does not.
      const commandName = unknownCommandName(childNode, readText)
      if (commandName) {
        const markup = markupMap.get(commandName)
        if (markup) {
          addMarkup(markup, childNode)
          return
        }

        if (['\\corref', '\\fnref', '\\thanks'].includes(commandName)) {
          // Marks that belong on the printed page and not in a heading.
          from = childNode.to
          return false
        }

        const symbol = COMMAND_SUBSTITUTIONS.get(commandName)
        if (symbol) {
          ancestor().append(document.createTextNode(symbol))
          from = childNode.to
          return false
        }
      } else if (childNode.type.is('LineBreak')) {
        ancestor().append(document.createElement('br'))
        from = childNode.to
      }
    },
    function leave(childNodeRef) {
      const childNode = childNodeRef.node

      if (shouldHandleLeave(childNode, readText)) {
        const typeSetElement = popAncestor()
        ancestor().appendChild(typeSetElement)
        const textArgument = childNode.getChild('TextArgument')
        const endBrace = textArgument?.getChild('CloseBrace')
        if (endBrace) {
          from = endBrace.to
        }
      }
    }
  )

  if (from < node.to) {
    ancestor().append(document.createTextNode(readText(from, node.to)))
  }

  return element
}

const chooseFrom = (node: SyntaxNode) =>
  node.getChild('TextArgument')?.getChild('LongArg')?.from ?? node.to

const shouldHandleLeave = (node: SyntaxNode, getText: (from: number, to: number) => string) => {
  if (textFormattingMarkupMap.has(node.type.name)) {
    return true
  }

  const commandName = unknownCommandName(node, getText)
  return Boolean(commandName && markupMap.has(commandName))
}

const unknownCommandName = (
  node: SyntaxNode,
  getText: (from: number, to: number) => string
): string | undefined => {
  if (node.type.is('UnknownCommand')) {
    const commandNameNode = node.getChild('$CtrlSeq')
    if (commandNameNode) {
      return getText(commandNameNode.from, commandNameNode.to).trim()
    }
  }
}
