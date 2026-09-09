import { useCallback } from 'react'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useEventListener } from '@/lib/hooks'

function scrollIntoView(element: Element) {
  setTimeout(() => {
    element.scrollIntoView({
      block: 'start',
      inline: 'nearest',
    })
  })
}

/**
 * Listens for the editor asking the logs pane to show one entry: opened
 * from a compile-log mark in the gutter.
 */
export const useLogEvents = (setShowLogs: (show: boolean) => void) => {
  const { pdfLayout, setView } = useLayout()

  const selectLogNewLogs = useCallback((id: string) => {
    window.setTimeout(() => {
      const logEntry = document.querySelector(`.log-entry[data-log-entry-id="${id}"]`)

      if (logEntry) {
        scrollIntoView(logEntry)

        const expandCollapseButton = logEntry.querySelector<HTMLElement>('[data-action="expand-collapse"]')

        const collapsed = expandCollapseButton?.dataset.collapsed === 'true'

        if (collapsed) {
          expandCollapseButton.click()
        }
      }
    })
  }, [])

  const openLogs = useCallback(() => {
    setShowLogs(true)

    if (pdfLayout === 'flat') {
      setView('pdf')
    }
  }, [pdfLayout, setView, setShowLogs])

  const handleViewCompileLogEntryEvent = useCallback(
    (event: Event) => {
      const { id } = (event as CustomEvent<{ id: string }>).detail

      openLogs()

      selectLogNewLogs(id)
    },
    [openLogs, selectLogNewLogs]
  )

  useEventListener('editor:view-compile-log-entry' as keyof WindowEventMap, handleViewCompileLogEntryEvent)
}
