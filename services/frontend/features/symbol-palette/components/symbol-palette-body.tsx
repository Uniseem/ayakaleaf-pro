import { useTranslation } from '@/lib/i18n'
import { SymbolPaletteItems } from './symbol-palette-items'
import type { Category, Symbol } from '../utils/categories'

/**
 * The panels under the tabs.
 *
 * While searching, the matches replace the categories -- but the empty panels
 * are still rendered, because each tab names its panel through aria-controls
 * and a tab pointing at nothing is worse than a tab pointing at an empty box.
 */
export function SymbolPaletteBody({
  categories,
  categorisedSymbols,
  filteredSymbols,
  handleSelect,
  focusInput,
  activeCategoryId,
}: {
  categories: Category[]
  categorisedSymbols: Record<string, Symbol[]>
  filteredSymbols: Symbol[] | null
  handleSelect: (symbol: Symbol) => void
  focusInput: () => void
  activeCategoryId: string
}) {
  const { t } = useTranslation()

  if (filteredSymbols) {
    return (
      <div className="symbol-palette-panels">
        {filteredSymbols.length ? (
          <SymbolPaletteItems
            items={filteredSymbols}
            handleSelect={handleSelect}
            focusInput={focusInput}
          />
        ) : (
          <div className="symbol-palette-empty">{t('no_symbols_found')}</div>
        )}

        {categories.map(category => (
          <div
            key={category.id}
            role="tabpanel"
            className="symbol-palette-panel"
            id={`symbol-palette-panel-${category.id}`}
            aria-labelledby={`symbol-palette-tab-${category.id}`}
            hidden
          />
        ))}
      </div>
    )
  }

  return (
    <div className="symbol-palette-panels">
      {categories.map(category => (
        <div
          key={category.id}
          id={`symbol-palette-panel-${category.id}`}
          className="symbol-palette-panel"
          role="tabpanel"
          aria-labelledby={`symbol-palette-tab-${category.id}`}
          hidden={category.id !== activeCategoryId}
        >
          <SymbolPaletteItems
            items={categorisedSymbols[category.id] ?? []}
            handleSelect={handleSelect}
            focusInput={focusInput}
          />
        </div>
      ))}
    </div>
  )
}
