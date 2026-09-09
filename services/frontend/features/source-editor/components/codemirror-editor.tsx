'use client'

/**
 * The editor: one CodeMirror view, created once, and the React pieces that
 * live around it.
 *
 * The view is made here and handed down through context. Every transaction
 * it processes also updates a piece of React state holding the editor state,
 * so the toolbar and search form re-render as the document and selection
 * change without owning the view themselves.
 */

import { memo, useRef, useState } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { CodeMirrorStateContext, CodeMirrorViewContext } from './codemirror-context'
import CodeMirrorView from './codemirror-view'
import { CodeMirrorToolbar } from './codemirror-toolbar'
import CodeMirrorSearch from './codemirror-search'
import { CodemirrorOutline } from './codemirror-outline'
import { useToolbarMenuBarEditorCommands } from '../hooks/use-toolbar-menu-editor-commands'
import { useFigureModalFallback } from '../hooks/use-figure-modal-fallback'
import { useIsMounted } from '@/lib/hooks'

function CodeMirrorEditor() {
  // create the initial state
  const [state, setState] = useState(() => {
    return EditorState.create()
  })

  const isMounted = useIsMounted()

  // create the view using the initial state and intercept transactions
  const viewRef = useRef<EditorView | null>(null)
  if (viewRef.current === null) {
    // EditContext-based editing is disabled until it is stable
    ;(EditorView as unknown as { EDIT_CONTEXT: boolean }).EDIT_CONTEXT = false

    const view = new EditorView({
      state,
      dispatchTransactions: trs => {
        view.update(trs)
        if (isMounted()) {
          setState(view.state)
        }
      },
    })
    viewRef.current = view
  }

  return (
    <CodeMirrorStateContext.Provider value={state}>
      <CodeMirrorViewContext.Provider value={viewRef.current}>
        <CodeMirrorEditorComponents />
      </CodeMirrorViewContext.Provider>
    </CodeMirrorStateContext.Provider>
  )
}

function CodeMirrorEditorComponents() {
  useToolbarMenuBarEditorCommands()
  useFigureModalFallback()

  return (
    <>
      <CodemirrorOutline />
      <CodeMirrorView />
      <CodeMirrorSearch />
      <CodeMirrorToolbar />
    </>
  )
}

export default memo(CodeMirrorEditor)
