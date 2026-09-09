import { useLayoutEffect, type RefObject } from 'react'
import type { ImperativePanelHandle } from 'react-resizable-panels'

/**
 * Keeps a resizable panel collapsed or expanded to match a flag, in the
 * same paint as the flag's other effects so nothing flashes in between.
 */
export function useCollapsiblePanel(panelIsOpen: boolean, panelRef: RefObject<ImperativePanelHandle | null>) {
  useLayoutEffect(() => {
    const panelHandle = panelRef.current
    if (panelHandle) {
      if (panelIsOpen) {
        panelHandle.expand()
      } else {
        panelHandle.collapse()
      }
    }
  }, [panelIsOpen, panelRef])
}
