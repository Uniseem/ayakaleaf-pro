import { Dispatch, SetStateAction, useCallback } from 'react'
import { useEventListener } from '@/lib/hooks'
import { isMac } from '@/lib/os'

/** Ctrl+P, or Cmd+P on a Mac, opens and closes the palette. */
const useCommandPaletteTriggers = (show: Dispatch<SetStateAction<boolean>>) => {
  const onKeyDown = useCallback(
    (event: KeyboardEvent) => {
      const modifierKey = isMac ? event.metaKey : event.ctrlKey
      if (modifierKey && event.code === 'KeyP') {
        event.preventDefault()
        show(prev => !prev)
      }
    },
    [show]
  )

  useEventListener('keydown', onKeyDown)
}

export default useCommandPaletteTriggers
