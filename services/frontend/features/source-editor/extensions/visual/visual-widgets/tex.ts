import { WidgetType } from '@codemirror/view'

/** The TeX logo, with its dropped E. */
export class TeXWidget extends WidgetType {
  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-tex')
    element.append('T', subscript('e'), 'X')
    return element
  }

  eq() {
    return true
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}

function subscript(text: string) {
  const element = document.createElement('sub')
  element.textContent = text
  return element
}
