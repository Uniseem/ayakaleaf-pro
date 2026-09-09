import type { CompletionContext } from '@codemirror/autocomplete'
import { buildIncludeCompletions } from './include'
import { buildLabelCompletions } from './labels'
import { buildPackageCompletions } from './packages'
import { buildSnippetCompletions } from './snippets'
import { buildEnvironmentCompletions } from './environments'
import { buildReferenceCompletions } from './references'
import { buildClassCompletions } from './classes'
import type { Completions } from './types'
import { buildBibliographyStyleCompletions } from './bibliography-styles'

export const buildAllCompletions = (completions: Completions, context: CompletionContext) => {
  buildSnippetCompletions(completions)
  buildEnvironmentCompletions(completions)
  buildClassCompletions(completions)
  buildBibliographyStyleCompletions(completions)
  buildIncludeCompletions(completions, context)
  buildReferenceCompletions(completions, context)
  buildLabelCompletions(completions, context)
  buildPackageCompletions(completions, context)

  return completions
}
