import { useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import customLocalStorage from '@/lib/local-storage'
import { debugConsole } from '@/lib/debug'
import type { PdfScrollPosition } from '../util/types'

export const usePdfScrollPosition = (
  lastCompileRootDocId: string | null | undefined
): [PdfScrollPosition | undefined, Dispatch<SetStateAction<PdfScrollPosition | undefined>>] => {
  // scroll position of the PDF
  const [position, setPosition] = useState<PdfScrollPosition | undefined>()

  const lastCompileRootDocIdRef = useRef<string | null | undefined>(lastCompileRootDocId)
  useEffect(() => {
    lastCompileRootDocIdRef.current = lastCompileRootDocId
  }, [lastCompileRootDocId])

  const initialScrollPositionRef = useRef<PdfScrollPosition | null>(null)

  // load the stored PDF scroll position when the compiled root doc changes
  useEffect(() => {
    if (lastCompileRootDocId) {
      const position = customLocalStorage.getItem(`pdf.position.${lastCompileRootDocId}`)
      if (position) {
        debugConsole.log('loaded position for', lastCompileRootDocId, position)
        initialScrollPositionRef.current = position
        setPosition(position)
      }
    }
  }, [lastCompileRootDocId])

  // store the current root doc's PDF scroll position when it changes
  useEffect(() => {
    if (lastCompileRootDocIdRef.current && position && position !== initialScrollPositionRef.current) {
      debugConsole.log('storing position for', lastCompileRootDocIdRef.current, position)
      customLocalStorage.setItem(`pdf.position.${lastCompileRootDocIdRef.current}`, position)
    }
  }, [position])

  return [position, setPosition]
}
