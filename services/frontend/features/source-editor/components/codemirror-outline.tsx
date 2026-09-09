'use client'

import { memo, useEffect } from 'react'
import { useCodeMirrorStateContext } from './codemirror-context'
import { documentOutline } from '../languages/latex/document-outline'
import { ProjectionStatus } from '../utils/tree-operations/projection'
import useDebounce from '../hooks/use-debounce'
import { useOutlineContext } from '@/features/ide/contexts/outline-context'

export const CodemirrorOutline = memo(function CodemirrorOutline() {
  const { setFlatOutline } = useOutlineContext()

  const state = useCodeMirrorStateContext()
  const debouncedState = useDebounce(state, 100)
  const outlineResult = debouncedState.field(documentOutline, false)

  // when the outline projection changes, calculate the flat outline
  useEffect(() => {
    if (outlineResult && outlineResult.status !== ProjectionStatus.Pending) {
      // A (potentially partial) outline is available.
      setFlatOutline({
        items: outlineResult.items.map(element => ({
          level: element.level,
          title: element.title,
          line: element.line,
        })),
        partial: outlineResult?.status === ProjectionStatus.Partial,
      })
    } else {
      setFlatOutline(undefined)
    }
  }, [outlineResult, setFlatOutline])

  return null
})
