import { EditorView } from '@codemirror/view'
import { EditableGraphicsWidget } from './editable-graphics'

/**
 * An inline graphic with an edit button.
 *
 * The width goes on the wrapper rather than the image, because the button is
 * positioned against the wrapper and would otherwise float away from the
 * picture it belongs to.
 */
export class EditableInlineGraphicsWidget extends EditableGraphicsWidget {
  updateElementData(element: HTMLElement) {
    element.dataset.filepath = this.filePath
    element.dataset.width = this.figureData?.width?.toString()
    if (this.figureData?.width) {
      element.style.width = `min(100%, ${this.figureData.width * 100}%)`
    } else {
      element.style.width = ''
    }
  }

  toDOM(view: EditorView) {
    this.destroyed = false
    const element = document.createElement('span')
    element.classList.add('ol-cm-graphics-inline-edit-wrapper')
    this.updateElementData(element)
    const inlineElement = document.createElement('span')
    inlineElement.classList.add('ol-cm-graphics-inline')
    this.renderGraphic(inlineElement, view)
    element.append(inlineElement)
    return element
  }

  updateDOM(element: HTMLElement, view: EditorView): boolean {
    const updated = super.updateDOM(element, view)
    if (!updated) {
      return false
    }
    // We need to make sure these are updated, as `renderGraphic` in the base
    // class will update them on the inner element.
    this.updateElementData(element)
    view.requestMeasure()
    return true
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }

  // We set the actual figure width on the span rather than the img element
  getFigureWidth(): string {
    return '100%'
  }
}
