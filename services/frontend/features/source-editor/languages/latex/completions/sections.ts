import type { CompletionContext, CompletionSection } from '@codemirror/autocomplete'

type SectionGenerator = (context: CompletionContext, type: string) => CompletionSection | string | undefined

/**
 * Where a completion goes in the list. Nothing registers a generator here
 * yet -- the original's came from optional modules -- so options stay in
 * one list.
 */
const sectionTitleGenerators: Array<SectionGenerator> = []

export function maybeGetSectionForOption(context: CompletionContext, type: string) {
  for (const generator of sectionTitleGenerators) {
    const section = generator(context, type)
    if (section !== undefined) {
      return section
    }
  }
  return undefined
}
