import { useCallback, useEffect, useRef } from 'react'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useProject } from '@/features/ide/contexts/project-context'

export type GotoLineOptions = {
  /** 1-based */
  gotoLine?: number
  gotoColumn?: number
  selectText?: string
  syncToPdf?: boolean
  keepCurrentView?: boolean
}

/**
 * Opens a document and puts the cursor on a line in it.
 *
 * The file is often not the one that is open -- a thesis is a dozen
 * chapters -- so the file is opened first and the jump waits for its text
 * to arrive. Jumping straight away would move the cursor in the document
 * being left.
 */
export function useOpenDocAtLine() {
  const editor = useEditor()
  const { entryById } = useProject()

  const pending = useRef<{ docId: string; options: GotoLineOptions } | null>(null)

  const jump = useCallback((options: GotoLineOptions) => {
    if (options.gotoLine === undefined) {
      return
    }
    window.dispatchEvent(
      new CustomEvent('ide:goto-line', {
        detail: {
          line: options.gotoLine - 1,
          column: options.gotoColumn,
          selectText: options.selectText,
          syncToPdf: options.syncToPdf,
        },
      })
    )
  }, [])

  const openDocWithId = useCallback(
    (docId: string, options: GotoLineOptions = {}) => {
      if (editor.current?.id === docId) {
        jump(options)
        return
      }
      const entry = entryById(docId)
      if (!entry || entry.kind !== 'doc') {
        return
      }
      pending.current = { docId, options }
      editor.open(entry)
    },
    [editor, entryById, jump]
  )

  // The file asked for has arrived. `loading` is the signal rather than the
  // id, which changes the moment the file is chosen and says nothing about
  // whether its text is here yet.
  const openId = editor.current?.id
  const stillLoading = editor.loading
  useEffect(() => {
    const wanted = pending.current
    if (!wanted || stillLoading || openId !== wanted.docId) {
      return
    }
    pending.current = null
    // the editor state for the new document is built on the next tick
    window.setTimeout(() => jump(wanted.options), 50)
  }, [openId, stillLoading, jump])

  return { openDocWithId }
}
