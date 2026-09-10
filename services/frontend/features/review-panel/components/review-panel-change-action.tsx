'use client'

import { memo, type MouseEventHandler, type ReactNode } from 'react'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'

/**
 * Stops a click on a card's button from also selecting the card.
 *
 * Selecting moves the cursor into the document, which scrolls, which moves
 * the card out from under the pointer mid-click.
 */
export function PreventSelectingEntry({ children }: { children: ReactNode }) {
  return (
    <div
      onMouseDown={event => event.stopPropagation()}
      onMouseUp={event => event.stopPropagation()}
      onClick={event => event.stopPropagation()}
    >
      {children}
    </div>
  )
}

export const ChangeAction = memo(function ChangeAction({
  id,
  label,
  type,
  handleClick,
}: {
  id: string
  label: string
  type: string
  handleClick: MouseEventHandler<HTMLButtonElement>
}) {
  return (
    <PreventSelectingEntry>
      <Tooltip
        id={id}
        description={label}
        overlayProps={{ placement: 'bottom' }}
        tooltipProps={{ className: 'review-panel-tooltip' }}
      >
        <button type="button" className="btn" onClick={handleClick} tabIndex={0}>
          <MaterialIcon type={type} className="review-panel-entry-actions-icon" accessibilityLabel={label} />
        </button>
      </Tooltip>
    </PreventSelectingEntry>
  )
})
