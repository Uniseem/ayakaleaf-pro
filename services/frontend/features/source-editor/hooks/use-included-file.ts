import { useCallback } from 'react'
import { EditorState } from '@codemirror/state'
import { useCodeMirrorStateContext } from '@/features/source-editor/components/codemirror-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useOpenDocAtLine } from '@/features/ide/hooks/use-open-doc-at-line'
import { resolveCommandNode } from '@/features/source-editor/extensions/command-tooltip'
import {
  FilePathArgument,
  LiteralArgContent,
} from '@/features/source-editor/lezer-latex/latex.terms.mjs'

/**
 * Opens the file an include command names.
 *
 * The .tex is tried first and then the bare name, because that is the order
 * LaTeX itself resolves them in -- a project holding both foo.tex and foo
 * compiles the former.
 */
export const useIncludedFile = (argumentType: string) => {
  const state = useCodeMirrorStateContext()
  const { entryByPath } = useProject()
  const { openDocWithId } = useOpenDocAtLine()

  const openIncludedFile = useCallback(() => {
    const name = readIncludedPath(state, argumentType)
    if (name) {
      for (const extension of ['.tex', '']) {
        const entry = entryByPath(`${name}${extension}`)
        if (entry) {
          return openDocWithId(entry.id)
        }
      }
    }
  }, [argumentType, entryByPath, openDocWithId, state])

  return { openIncludedFile }
}

const readIncludedPath = (state: EditorState, argumentType: string | number) => {
  const commandNode = resolveCommandNode(state)
  const argumentNode = commandNode
    ?.getChild(argumentType)
    ?.getChild(FilePathArgument)
    ?.getChild(LiteralArgContent)

  if (argumentNode) {
    return state.sliceDoc(argumentNode.from, argumentNode.to)
  }
}
