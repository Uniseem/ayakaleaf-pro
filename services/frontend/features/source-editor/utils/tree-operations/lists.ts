import { EditorState } from '@codemirror/state'
import { SyntaxNode } from '@lezer/common'

/**
 * Which kind of list an environment is.
 *
 * Both ends are checked: an itemize whose closing tag says something else is
 * a document mid-edit, and guessing its type would renumber the items under
 * the writer as they type.
 */
export const getListType = (
  state: EditorState,
  listEnvironmentNode: SyntaxNode
) => {
  const beginEnvNameNode = listEnvironmentNode
    .getChild('BeginEnv')
    ?.getChild('EnvNameGroup')
    ?.getChild('ListEnvName')

  const endEnvNameNode = listEnvironmentNode
    .getChild('EndEnv')
    ?.getChild('EnvNameGroup')
    ?.getChild('ListEnvName')

  if (beginEnvNameNode && endEnvNameNode) {
    return state.sliceDoc(beginEnvNameNode.from, beginEnvNameNode.to).trim()
  }
}
