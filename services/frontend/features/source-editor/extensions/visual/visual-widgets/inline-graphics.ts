import { EditorView } from '@codemirror/view'
import { GraphicsWidget } from './graphics'

/**
 * The same graphic, drawn inside a line rather than as a block.
 *
 * `\includegraphics` outside a figure environment sits in the text, so it
 * cannot take a block widget's clicks -- hence the different event handling.
 */
export class InlineGraphicsWidget extends GraphicsWidget {
  toDOM(view: EditorView) {
    this.destroyed = false

    const element = document.createElement('span')
    element.classList.add('ol-cm-graphics-inline')

    this.renderGraphic(element, view)

    return element
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
