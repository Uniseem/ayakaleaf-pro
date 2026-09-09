'use client'

import { createPortal } from 'react-dom'
import { getPanel } from '@codemirror/view'
import { createSearchPanel } from '@codemirror/search'
import CodeMirrorSearchForm from './codemirror-search-form'
import { useCodeMirrorViewContext } from './codemirror-context'

function CodeMirrorSearch() {
  const view = useCodeMirrorViewContext()

  const panel = getPanel(view, createSearchPanel)

  if (!panel) {
    return null
  }

  return createPortal(<CodeMirrorSearchForm />, panel.dom)
}

export default CodeMirrorSearch
