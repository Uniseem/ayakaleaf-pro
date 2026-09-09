import { useCallback, useEffect, useRef, useState } from 'react'

/** Open state for a popover that closes on a click anywhere outside it. */
export default function useDropdown(defaultOpen = false) {
  const [open, setOpen] = useState(defaultOpen)

  // the dropdown node, for use in the "click outside" event listener
  const ref = useRef<Element | null>(null)

  const handleRef = useCallback((node: Element | null) => {
    ref.current = node
  }, [])

  // prevent a click on the dropdown toggle propagating to the original handler
  const handleClick = useCallback((event: React.SyntheticEvent) => {
    event.stopPropagation()
  }, [])

  // handle dropdown toggle
  const handleToggle = useCallback((value: unknown) => {
    setOpen(Boolean(value))
  }, [])

  // close the dropdown on click outside the dropdown
  const handleDocumentClick = useCallback((event: MouseEvent) => {
    if (ref.current && !ref.current.contains(event.target as Node)) {
      setOpen(false)
    }
  }, [])

  // add/remove listener for click anywhere in document
  useEffect(() => {
    if (open) {
      document.addEventListener('mousedown', handleDocumentClick)
    }

    return () => {
      document.removeEventListener('mousedown', handleDocumentClick)
    }
  }, [open, handleDocumentClick])

  return { ref: handleRef, onClick: handleClick, onToggle: handleToggle, open }
}
