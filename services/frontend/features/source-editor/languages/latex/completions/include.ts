import type { CompletionContext } from '@codemirror/autocomplete'
import { extendOverUnpairedClosingBrace, extendRequiredParameter } from './apply'
import type { Completions } from './types'
import { metadataState } from '../../../extensions/language'

function removeBibExtension(path: string) {
  return path.replace(/\.bib$/, '')
}

function removeTexExtension(path: string) {
  return path.replace(/\.tex$/, '')
}

/**
 * Completions based on files in the project
 */
export function buildIncludeCompletions(completions: Completions, context: CompletionContext) {
  const metadata = context.state.field(metadataState, false)

  if (!metadata?.fileTreeData) {
    return
  }

  // files in the project folder
  const processFile = (path: string) => {
    if (/\.(?:tex|txt)$/.test(path)) {
      // path parameter for \include{path} or \input{path}
      completions.includes!.push({
        type: 'file',
        label: path,
        apply: removeTexExtension(path),
        extend: extendRequiredParameter,
      })

      // \include{path}
      completions.commands!.push({
        type: 'cmd',
        label: `\\include{${path}}`,
        apply: `\\include{${removeTexExtension(path)}}`,
        extend: extendOverUnpairedClosingBrace,
      })

      // \input{path}
      completions.commands!.push({
        type: 'cmd',
        label: `\\input{${path}}`,
        apply: `\\input{${removeTexExtension(path)}}`,
        extend: extendOverUnpairedClosingBrace,
      })

      // \subfile{path}
      completions.commands!.push({
        type: 'cmd',
        label: `\\subfile{${path}}`,
        apply: `\\subfile{${removeTexExtension(path)}}`,
        extend: extendOverUnpairedClosingBrace,
      })
    }

    if (/\.(eps|jpe?g|gif|png|tiff?|pdf|svg)$/i.test(path)) {
      // path parameter for \includegraphics{path}
      completions.graphics!.push({
        type: 'file',
        label: path,
        extend: extendRequiredParameter,
      })

      completions.commands!.push({
        type: 'cmd',
        label: `\\includegraphics{${path}}`,
        extend: extendOverUnpairedClosingBrace,
      })
    }

    if (/\.bib$/.test(path)) {
      const label = removeBibExtension(path)
      // path without extension for \bibliography{path}
      completions.bibliographies!.push({
        type: 'bib',
        label,
        extend: extendRequiredParameter,
      })
    }
  }

  // The tree is held flat, each entry carrying its full path.
  for (const entry of metadata.fileTreeData) {
    if (entry.kind === 'folder') {
      continue
    }
    processFile(entry.path.replace(/^\//, ''))
  }
}
