import { useCallback, useEffect, useRef, useState } from 'react'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useOpenDocAtLine } from '@/features/ide/hooks/use-open-doc-at-line'
import { syncFromCode, syncFromPdf } from '@/lib/editor'
import { useEventListener, useIsMounted } from '@/lib/hooks'
import customLocalStorage from '@/lib/local-storage'
import { debugConsole } from '@/lib/debug'
import { isValidTeXFile } from '@/features/source-editor/utils/misc'
import { showFileErrorToast, showSynctexRequestErrorToast } from '../components/synctex-toasts'
import type { PdfScrollPosition } from '../util/types'

type CursorPosition = { row: number; column: number }

const dirnameOf = (path: string) => {
  const trimmed = path.replace(/^\//, '')
  const index = trimmed.lastIndexOf('/')
  return index === -1 ? '' : trimmed.slice(0, index)
}

/**
 * Both directions of SyncTeX: from the cursor to the place in the PDF, and
 * from a place in the PDF to the line that produced it.
 */
export default function useSynctex(): {
  syncToPdf: () => void
  syncToCode: ({ visualOffset }: { visualOffset?: number }) => void
  syncToPdfInFlight: boolean
  syncToCodeInFlight: boolean
  canSyncToPdf: boolean
} {
  const { projectId, project, files, entryByPath } = useProject()
  const rootDocId = project?.rootDocId

  const { position, setShowLogs, setHighlights } = useCompile()

  const editor = useEditor()
  const openDocName = editor.current?.name
  const currentDocId = editor.current?.id
  const currentDocPath = editor.current?.path
  const { openDocWithId } = useOpenDocAtLine()

  const [cursorPosition, setCursorPosition] = useState<CursorPosition | null>(() => {
    if (!currentDocId) {
      return null
    }
    const position = customLocalStorage.getItem(`doc.position.${currentDocId}`)
    return position ? position.cursorPosition : null
  })

  const isMounted = useIsMounted()

  useEventListener(
    'cursor:editor:update' as keyof WindowEventMap,
    useCallback((event: Event) => setCursorPosition((event as CustomEvent<CursorPosition>).detail), [])
  )

  const [syncToPdfInFlight, setSyncToPdfInFlight] = useState(false)
  const [syncToCodeInFlight, setSyncToCodeInFlight] = useState(false)

  const getCurrentFilePath = useCallback(() => {
    if (!currentDocPath || !rootDocId) {
      return null
    }

    let path = currentDocPath.replace(/^\//, '')

    // If the root file is folder/main.tex, then synctex sees the path as folder/./main.tex
    const rootDoc = files.find(entry => entry.id === rootDocId)
    const rootDocDirname = rootDoc ? dirnameOf(rootDoc.path) : ''

    if (rootDocDirname) {
      path = path.replace(RegExp(`^${rootDocDirname}`), `${rootDocDirname}/.`)
    }

    return path
  }, [currentDocPath, rootDocId, files])

  const goToCodeLine = useCallback(
    (file?: string, line?: number, selectText?: string) => {
      if (file) {
        const path = file.replace(/^\.\//, '')
        const doc = entryByPath(path) ?? entryByPath('/' + path)
        if (doc && doc.kind === 'doc') {
          openDocWithId(doc.id, {
            gotoLine: line,
            selectText,
          })
          return
        }
      }
      showFileErrorToast()
    },
    [entryByPath, openDocWithId]
  )

  const goToPdfLocation = useCallback(
    (file: string, line: number, column: number) => {
      setSyncToPdfInFlight(true)

      syncFromCode(projectId, file, line, column)
        .then(data => {
          setShowLogs(false)
          setHighlights(data.pdf)
        })
        .catch(error => {
          showSynctexRequestErrorToast()
          debugConsole.error(error)
        })
        .finally(() => {
          if (isMounted()) {
            setSyncToPdfInFlight(false)
          }
        })
    },
    [isMounted, projectId, setShowLogs, setHighlights]
  )

  const cursorPositionRef = useRef(cursorPosition)

  useEffect(() => {
    cursorPositionRef.current = cursorPosition
  }, [cursorPosition])

  const syncToPdf = useCallback(() => {
    const file = getCurrentFilePath()

    if (cursorPositionRef.current) {
      const { row, column } = cursorPositionRef.current
      goToPdfLocation(file ?? '', row + 1, column)
    }
  }, [getCurrentFilePath, goToPdfLocation])

  useEventListener(
    'cursor:editor:syncToPdf' as keyof WindowEventMap,
    useCallback(() => {
      syncToPdf()
    }, [syncToPdf])
  )

  const positionRef = useRef(position)
  useEffect(() => {
    positionRef.current = position
  }, [position])

  const syncToCode = useCallback(
    ({
      position = positionRef.current,
      selectText,
      visualOffset = 0,
    }: {
      position?: PdfScrollPosition
      selectText?: string
      visualOffset?: number
    }) => {
      if (!position) {
        return
      }

      setSyncToCodeInFlight(true)
      // This works better if it's halfway across the page (or the visible
      // part of the page). Synctex doesn't always find the right place in the
      // file when the point is at the edge of the page; it sometimes returns
      // the start of the next paragraph instead.
      const h = position.offset.left

      // Compute the vertical position to pass to synctex, which works with
      // coordinates increasing from the top of the page down. This matches
      // the browser's DOM coordinate of the click point, but the pdf position
      // is measured from the bottom of the page so it needs inverting.
      let v = 0
      if (position.pageSize?.height) {
        v += position.pageSize.height - position.offset.top // measure from pdf point (inverted)
      } else {
        v += position.offset.top // measure from html click position
      }
      v += visualOffset

      syncFromPdf(projectId, position.page + 1, Number(h.toFixed(2)), Number(v.toFixed(2)))
        .then(data => {
          const first = data.code[0]
          goToCodeLine(first?.file, first?.line, selectText)
        })
        .catch(error => {
          debugConsole.error(error)
          showSynctexRequestErrorToast()
        })
        .finally(() => {
          if (isMounted()) {
            setSyncToCodeInFlight(false)
          }
        })
    },
    [projectId, isMounted, goToCodeLine]
  )

  useEventListener(
    'synctex:sync-to-position' as keyof WindowEventMap,
    useCallback(
      (event: Event) => syncToCode((event as CustomEvent<{ position: PdfScrollPosition; selectText?: string }>).detail),
      [syncToCode]
    )
  )

  const hasSingleSelectedDoc = Boolean(currentDocId) && !editor.currentFile

  const canSyncToPdf: boolean = Boolean(hasSingleSelectedDoc && cursorPosition && openDocName && isValidTeXFile(openDocName))

  return {
    syncToCode,
    syncToPdf,
    syncToPdfInFlight,
    syncToCodeInFlight,
    canSyncToPdf,
  }
}
