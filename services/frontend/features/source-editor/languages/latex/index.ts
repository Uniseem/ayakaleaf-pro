import { LanguageSupport } from '@codemirror/language'
import type { CompletionSource } from '@codemirror/autocomplete'
import { latexIndentService } from './latex-indent-service'
import { shortcuts } from './shortcuts'
import { linting } from './linting'
import { openAutocomplete } from './open-autocomplete'
import { metadata } from './metadata'
import {
  argumentCompletionSources,
  explicitCommandCompletionSource,
  inCommandCompletionSource,
  beginEnvironmentCompletionSource,
} from './complete'
import { documentCommands } from './document-commands'
import { documentOutline } from './document-outline'
import { LaTeXLanguage } from './latex-language'
import { documentEnvironments } from './document-environments'

const completionSources: CompletionSource[] = [
  ...argumentCompletionSources,
  inCommandCompletionSource,
  explicitCommandCompletionSource,
  beginEnvironmentCompletionSource,
]

export const latex = () => {
  return new LanguageSupport(LaTeXLanguage, [
    shortcuts(),
    documentOutline,
    documentCommands,
    documentEnvironments,
    latexIndentService(),
    linting(),
    metadata(),
    openAutocomplete(),
    ...completionSources.map(completionSource =>
      LaTeXLanguage.data.of({
        autocomplete: completionSource,
      })
    ),
  ])
}
