'use client'

import MaterialIcon from '@/components/ol/material-icon'

/**
 * The small mark in the gutter beside a card.
 *
 * Shown when the panel is collapsed to its narrow form, where the cards
 * themselves are off screen and this is all there is to say something is here.
 */
export function EntryIndicator({
  type,
  handleMouseEnter,
  handleMouseLeave,
  handleMouseDown,
}: {
  type: 'comment' | 'edit'
  handleMouseEnter?: () => void
  handleMouseLeave?: () => void
  handleMouseDown?: () => void
}) {
  return (
    <div
      className="review-panel-entry-indicator"
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      onMouseDown={handleMouseDown}
    >
      <MaterialIcon type={type === 'comment' ? 'chat' : 'edit'} className="review-panel-entry-icon" />
    </div>
  )
}

export default EntryIndicator
