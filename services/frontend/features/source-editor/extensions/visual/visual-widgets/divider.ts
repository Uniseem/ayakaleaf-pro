import { WidgetType } from '@codemirror/view'

/** The rule drawn where a document is divided. */
export class DividerWidget extends WidgetType {
  toDOM() {
    const element = document.createElement('div')
    element.classList.add('ol-cm-divider')
    return element
  }

  eq() {
    return true
  }

  updateDOM(): boolean {
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
