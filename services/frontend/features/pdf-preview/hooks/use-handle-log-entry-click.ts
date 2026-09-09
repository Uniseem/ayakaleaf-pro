import { useCallback, type MouseEventHandler } from 'react'
import type { ErrorLevel, SourceLocation } from '../util/types'

const useHandleLogEntryClick = ({
  sourceLocation,
  onSourceLocationClick,
}: {
  level: ErrorLevel | undefined
  ruleId: string | undefined
  sourceLocation: SourceLocation | undefined
  onSourceLocationClick?: (location: SourceLocation) => void
}) => {
  const handleLogEntryLinkClick: MouseEventHandler<HTMLButtonElement> = useCallback(
    event => {
      event.preventDefault()

      if (onSourceLocationClick && sourceLocation) {
        onSourceLocationClick(sourceLocation)
      }
    },
    [onSourceLocationClick, sourceLocation]
  )

  return handleLogEntryLinkClick
}

export default useHandleLogEntryClick
