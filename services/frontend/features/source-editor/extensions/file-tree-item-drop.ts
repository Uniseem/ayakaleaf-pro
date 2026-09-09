import { EditorView } from '@codemirror/view'
import { EditorSelection } from '@codemirror/state'
import { snippet } from '@codemirror/autocomplete'
import { hasImageExtension } from '../utils/misc'
import { prepareSnippetTemplate } from '../languages/latex/snippets'

/** Drag a file from the file tree into the text to reference it. */
export const fileTreeItemDrop = () =>
  EditorView.domEventHandlers({
    dragover(event) {
      if (event.dataTransfer) {
        event.preventDefault()
      }
    },
    drop(event, view) {
      if (event.dataTransfer) {
        const fileId = event.dataTransfer.getData('application/x-overleaf-file-id')

        const filePath = event.dataTransfer.getData('application/x-overleaf-file-path')

        if (fileId && filePath) {
          event.preventDefault()

          const pos = view.posAtCoords(event)
          if (pos !== null) {
            handleDroppedFile(view, pos, filePath)
          }
        }
      }
    },
  })

const withoutExtension = (filename: string) => filename.substring(0, filename.lastIndexOf('.'))

const handleDroppedFile = (view: EditorView, pos: number, filePath: string) => {
  if (filePath.endsWith('.bib')) {
    view.focus()

    const insert = `\\bibliography{${withoutExtension(filePath)}}`
    view.dispatch({
      changes: { from: pos, insert },
      selection: EditorSelection.cursor(pos + insert.length),
    })

    return
  }

  if (filePath.endsWith('.tex')) {
    view.focus()

    const insert = `\\input{${withoutExtension(filePath)}}`
    view.dispatch({
      changes: { from: pos, insert },
      selection: EditorSelection.cursor(pos + insert.length),
    })

    return
  }

  if (hasImageExtension(filePath)) {
    view.focus()

    const template = prepareSnippetTemplate(
      `\\begin{figure}
\t\\centering
\t\\includegraphics[width=0.5\\linewidth]{${filePath}}
\t\\caption{$1}
\t\\label{$2}
\\end{figure}`
    )
    snippet(template)(view, { label: 'Figure' }, pos, pos)
  }
}
