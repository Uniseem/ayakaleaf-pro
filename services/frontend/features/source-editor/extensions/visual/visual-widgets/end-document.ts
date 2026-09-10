import { EditorView, WidgetType } from '@codemirror/view'
import { placeSelectionInsideBlock } from '../selection'

/**
 * The marker drawn where the document ends.
 *
 * Anything below it is not typeset, and without a marker it reads as simply
 * the next paragraph.
 */
export class EndDocumentWidget extends WidgetType {
  toDOM(view: EditorView): HTMLElement {
    const element = document.createElement('div')
    element.classList.add('ol-cm-end-document-widget')
    element.textContent = view.state.phrase('end_of_document')
    element.addEventListener('mouseup', event => {
      event.preventDefault()
      view.dispatch(placeSelectionInsideBlock(view, event as MouseEvent))
    })
    return element
  }

  ignoreEvent(event: Event): boolean {
    return event.type !== 'mouseup'
  }

  eq(): boolean {
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }

  get estimatedHeight() {
    return 30
  }
}
