import { useRef } from 'react'
import type { Category } from '../utils/categories'

/**
 * The category tabs.
 *
 * A tablist, so the arrow keys move between the categories and only the
 * selected tab is in the tab order -- which is what a screen reader and a
 * keyboard user both expect of tabs, and is not what a row of buttons gives.
 */
export function SymbolPaletteTabs({
  categories,
  activeCategoryId,
  setActiveCategoryId,
}: {
  categories: Category[]
  activeCategoryId: string
  setActiveCategoryId: (id: string) => void
}) {
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])

  const focusTab = (index: number) => {
    setActiveCategoryId(categories[index]!.id)
    buttonRefs.current[index]?.focus()
  }

  const handleKeyDown = (e: React.KeyboardEvent, index: number) => {
    switch (e.key) {
      case 'ArrowRight':
        focusTab((index + 1) % categories.length)
        break
      case 'ArrowLeft':
        focusTab((index - 1 + categories.length) % categories.length)
        break
      case 'Home':
      case 'PageUp':
        focusTab(0)
        break
      case 'End':
      case 'PageDown':
        focusTab(categories.length - 1)
        break
      default:
        break
    }
  }

  return (
    <div
      role="tablist"
      aria-label="Symbol Categories"
      className="symbol-palette-tab-list"
      tabIndex={0}
    >
      {categories.map((category, index) => {
        const selected = activeCategoryId === category.id
        return (
          <button
            key={category.id}
            role="tab"
            type="button"
            className="symbol-palette-tab"
            id={`symbol-palette-tab-${category.id}`}
            aria-controls={`symbol-palette-panel-${category.id}`}
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            ref={el => {
              buttonRefs.current[index] = el
            }}
            onClick={() => setActiveCategoryId(category.id)}
            onKeyDown={e => handleKeyDown(e, index)}
          >
            {category.label}
          </button>
        )
      })}
    </div>
  )
}
