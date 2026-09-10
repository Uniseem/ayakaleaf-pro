import symbols from '../data/symbols.json'

export type Symbol = {
  category: string
  command: string
  codepoint: string
  description: string
  aliases?: string[]
  notes?: string
  character: string
}

export type Category = {
  id: string
  label: string
}

export function createCategories(t: (key: string) => string): Category[] {
  return [
    {
      id: 'Greek',
      label: t('category_greek'),
    },
    {
      id: 'Arrows',
      label: t('category_arrows'),
    },
    {
      id: 'Operators',
      label: t('category_operators'),
    },
    {
      id: 'Relations',
      label: t('category_relations'),
    },
    {
      id: 'Misc',
      label: t('category_misc'),
    },
  ]
}

/**
 * The symbols to show under each tab.
 *
 * The character is derived from the codepoint rather than stored: the data
 * file is edited by hand, and a stored character would be a second place for
 * the same fact to be wrong.
 */
export function buildCategorisedSymbols(
  categories: Category[]
): Record<string, Symbol[]> {
  const output: Record<string, Symbol[]> = {}

  for (const category of categories) {
    output[category.id] = []
  }

  for (const item of symbols as Omit<Symbol, 'character'>[]) {
    if (item.category in output) {
      const character = String.fromCodePoint(
        parseInt(item.codepoint.replace(/^U\+0*/, ''), 16)
      )
      output[item.category]!.push({ ...item, character })
    }
  }

  return output
}

/** Every symbol, with its character, for searching across categories. */
export const allSymbols: Symbol[] = (symbols as Omit<Symbol, 'character'>[]).map(
  item => ({
    ...item,
    character: String.fromCodePoint(
      parseInt(item.codepoint.replace(/^U\+0*/, ''), 16)
    ),
  })
)
