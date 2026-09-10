import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useEditorPropertiesContext } from '@/features/ide/contexts/editor-properties-context'
import { SymbolPaletteItem } from './symbol-palette-item'
import type { Symbol } from '../utils/categories'

/**
 * A grid of symbols, crossed with the arrow keys.
 *
 * Up and down are worked out from the rendered geometry rather than from a
 * fixed column count: the grid wraps to the width of the pane, so how many
 * symbols are on a row is not known until they are laid out. A key that is not
 * one of these sends focus back to the search field, so typing while the grid
 * has focus searches rather than doing nothing.
 */
export function SymbolPaletteItems({
  items,
  handleSelect,
  focusInput,
}: {
  items: Symbol[]
  handleSelect: (symbol: Symbol) => void
  focusInput: () => void
}) {
  const [focusedIndex, setFocusedIndex] = useState(0)
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([])

  useEffect(() => {
    itemRefs.current = items.map((_, i) => itemRefs.current[i] || null)
    setFocusedIndex(0)
  }, [items])

  const { toggleSymbolPalette } = useEditorPropertiesContext()

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLButtonElement>) => {
      if (event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return

      const rects = itemRefs.current.map(ref => ref?.getBoundingClientRect() ?? null)
      const currentRect = rects[focusedIndex]
      if (!currentRect) return

      let newIndex = focusedIndex

      switch (event.key) {
        case 'ArrowLeft':
          newIndex = focusedIndex > 0 ? focusedIndex - 1 : items.length - 1
          break
        case 'ArrowRight':
          newIndex = focusedIndex < items.length - 1 ? focusedIndex + 1 : 0
          break
        case 'ArrowUp':
        case 'ArrowDown': {
          const direction = event.key === 'ArrowUp' ? -1 : 1
          const candidates = rects
            .map((rect, i) => ({ rect, i }))
            .filter(
              ({ rect }, i) =>
                i !== focusedIndex &&
                rect !== null &&
                Math.abs(rect.x - currentRect.x) < currentRect.width * 0.8 &&
                (direction === -1 ? rect.y < currentRect.y : rect.y > currentRect.y)
            )

          if (candidates.length > 0) {
            const closest = candidates.reduce((a, b) =>
              Math.abs(b.rect!.y - currentRect.y) < Math.abs(a.rect!.y - currentRect.y)
                ? b
                : a
            )
            newIndex = closest.i
          }
          break
        }
        case 'Home':
          newIndex = 0
          break
        case 'End':
          newIndex = items.length - 1
          break
        case 'Enter':
        case ' ':
          handleSelect(items[focusedIndex]!)
          toggleSymbolPalette()
          break
        case 'Escape':
          toggleSymbolPalette()
          window.dispatchEvent(new Event('editor:focus'))
          break

        default:
          focusInput()
          return
      }

      event.preventDefault()
      setFocusedIndex(newIndex)
    },
    [focusedIndex, items, focusInput, handleSelect, toggleSymbolPalette]
  )

  return (
    <div className="symbol-palette-items" role="listbox" aria-label="Symbols">
      {items.map((symbol, index) => (
        <SymbolPaletteItem
          key={symbol.codepoint}
          symbol={symbol}
          handleSelect={symbol => {
            handleSelect(symbol)
            setFocusedIndex(index)
          }}
          handleKeyDown={handleKeyDown}
          focused={index === focusedIndex}
          ref={el => {
            itemRefs.current[index] = el
          }}
        />
      ))}
    </div>
  )
}
