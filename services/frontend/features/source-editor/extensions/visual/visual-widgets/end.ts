import { WidgetType } from '@codemirror/view'

/** The space an environment's closing line leaves behind. */
export class EndWidget extends WidgetType {
  toDOM() {
    const element = document.createElement('div')
    element.classList.add('ol-cm-end')
    return element
  }

  eq() {
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
