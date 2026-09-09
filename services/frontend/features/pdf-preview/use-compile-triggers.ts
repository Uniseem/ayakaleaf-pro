import { useCallback, useEffect } from 'react'
import { useEventListener } from '@/lib/hooks'

/** Whether a keypress is one of the ones that start a compile. */
export const startCompileKeypress = (event: KeyboardEvent | React.KeyboardEvent<Element>) => {
  if (event.shiftKey || event.altKey) {
    return false
  }

  if (event.ctrlKey) {
    // Ctrl+s / Ctrl+Enter / Ctrl+.
    if (event.key === 's' || event.key === 'Enter' || event.key === '.') {
      return true
    }

    // Ctrl+s with Caps-Lock on
    if (event.key === 'S' && !event.shiftKey) {
      return true
    }
  } else if (event.metaKey) {
    // Cmd+s / Cmd+Enter
    if (event.key === 's' || event.key === 'Enter') {
      return true
    }

    // Cmd+s with Caps-Lock on
    if (event.key === 'S' && !event.shiftKey) {
      return true
    }
  }
  return false
}

/**
 * The keyboard shortcuts and events that start a compile, wherever the
 * focus is: the editor filters the same keys out of its own keymap so they
 * reach here.
 */
export default function useCompileTriggers(startCompile: () => void) {
  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (startCompileKeypress(event)) {
        event.preventDefault()
        startCompile()
      }
    },
    [startCompile]
  )

  const handleStartCompile = useCallback(() => {
    startCompile()
  }, [startCompile])
  useEventListener('pdf:recompile' as keyof WindowEventMap, handleStartCompile)

  useEffect(() => {
    document.body.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.removeEventListener('keydown', handleKeyDown)
    }
  }, [handleKeyDown])
}
