import { WidgetType } from '@codemirror/view'

/**
 * The non-breaking space a tilde stands for.
 *
 * A real no-break space rather than markup, so selecting across it copies a
 * space and the line breaks where LaTeX would break it.
 */
export class TildeWidget extends WidgetType {
  toDOM() {
    const element = document.createElement('span')
    element.textContent = ' '
    return element
  }

  eq() {
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
