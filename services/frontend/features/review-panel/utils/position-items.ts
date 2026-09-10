/**
 * Where the entry cards sit, from review-panel/utils/position-items.
 *
 * Each card wants to be level with the line it is about, and cards cannot
 * overlap. So one card is chosen as the fixed point -- the selected one, or
 * the one the cursor is inside, or the last one that had focus -- and the
 * rest are pushed away from it in both directions. Anchoring on the card the
 * reader is looking at is what keeps it still while the others move.
 */

import { debounce } from '@/lib/timing'

export const OFFSET_FOR_ENTRIES_ABOVE = 70
const GAP_BETWEEN_ENTRIES = 4

export const positionItems = debounce(
  (element: HTMLDivElement, focusedItemIndexRef: { current: number }, docId: string) => {
    const items = Array.from(element.querySelectorAll<HTMLDivElement>('.review-panel-entry'))

    items.sort((a, b) => Number(a.dataset.pos) - Number(b.dataset.pos))

    if (!items.length) {
      return
    }

    let activeItemIndex = items.findIndex(item =>
      item.classList.contains('review-panel-entry-selected')
    )

    if (activeItemIndex === -1) {
      // Nothing was picked by hand, so the one the cursor is inside stands in.
      activeItemIndex = items.findIndex(item =>
        item.classList.contains('review-panel-entry-highlighted')
      )
    }

    if (activeItemIndex === -1) {
      activeItemIndex = focusedItemIndexRef.current || 0
    }

    const activeItem = items[activeItemIndex]
    if (!activeItem) {
      return
    }

    const activeItemTop = getTopPosition(activeItem, activeItemIndex === 0)

    const positions: [HTMLElement, number][] = []
    positions.push([activeItem, activeItemTop])

    // Above the fixed point, walking up.
    let topLimit = activeItemTop
    for (let i = activeItemIndex - 1; i >= 0; i--) {
      const item = items[i]
      if (!item) continue
      const height = item.offsetHeight
      let top = getTopPosition(item, i === 0)
      const bottom = top + height
      if (bottom > topLimit) {
        top = topLimit - height - GAP_BETWEEN_ENTRIES
      }
      positions.push([item, top])
      topLimit = top
    }

    // Below it, walking down.
    let bottomLimit = activeItemTop + activeItem.offsetHeight
    for (let i = activeItemIndex + 1; i < items.length; i++) {
      const item = items[i]
      if (!item) continue
      const height = item.offsetHeight
      let top = getTopPosition(item, false)
      if (top < bottomLimit) {
        top = bottomLimit + GAP_BETWEEN_ENTRIES
      }
      positions.push([item, top])
      bottomLimit = top + height
    }

    for (const [item, top] of positions) {
      item.style.top = `${top}px`
      item.style.visibility = 'visible'
    }

    // Remembered so that the next pass anchors on the same card when
    // nothing is selected and the cursor is nowhere near one.
    focusedItemIndexRef.current = activeItemIndex
    void docId
  },
  100
)

function getTopPosition(item: HTMLDivElement, isFirstEntry: boolean) {
  const offset = isFirstEntry ? 0 : OFFSET_FOR_ENTRIES_ABOVE

  return Math.max(offset, Number(item.dataset.top))
}
