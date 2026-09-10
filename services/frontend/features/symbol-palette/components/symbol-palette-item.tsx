import { forwardRef, useEffect, useRef, type KeyboardEvent } from 'react'
import { Tooltip as OLTooltip } from '@/components/ol/tooltip'
import type { Symbol } from '../utils/categories'

/**
 * One symbol, as a button in a listbox.
 *
 * Only the focused item is in the tab order; the arrow keys move between them,
 * which is how a grid of a hundred buttons stays crossable by keyboard without
 * a hundred tab stops. The focus is only taken when focus is already inside
 * the grid, so opening the palette leaves the cursor in the search field.
 */
export const SymbolPaletteItem = forwardRef<
  HTMLButtonElement,
  {
    focused: boolean
    handleSelect: (symbol: Symbol) => void
    handleKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void
    symbol: Symbol
  }
>(function SymbolPaletteItem({ focused, handleSelect, handleKeyDown, symbol }, ref) {
  const buttonRef = useRef<HTMLButtonElement | null>(null)

  // Forward internal ref to parent
  useEffect(() => {
    if (ref) {
      if (typeof ref === 'function') {
        ref(buttonRef.current)
      } else {
        ref.current = buttonRef.current
      }
    }
  }, [ref])

  // Focus the item when it becomes focused
  useEffect(() => {
    if (
      focused &&
      buttonRef.current &&
      document.activeElement?.closest('.symbol-palette-items')
    ) {
      buttonRef.current.focus()
    }
  }, [focused])

  return (
    <OLTooltip
      id={`symbol-${symbol.codepoint}`}
      description={
        <div>
          <div className="symbol-palette-item-description">{symbol.description}</div>
          <div className="symbol-palette-item-command">{symbol.command}</div>
          {symbol.notes && (
            <div className="symbol-palette-item-notes">{symbol.notes}</div>
          )}
        </div>
      }
      overlayProps={{ placement: 'top' }}
    >
      <button
        className="symbol-palette-item"
        onClick={() => handleSelect(symbol)}
        onKeyDown={handleKeyDown}
        tabIndex={focused ? 0 : -1}
        ref={buttonRef}
        role="option"
        aria-label={symbol.description}
        aria-selected={focused ? 'true' : 'false'}
      >
        {symbol.character}
      </button>
    </OLTooltip>
  )
})
