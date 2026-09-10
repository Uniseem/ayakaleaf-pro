import { WidgetType } from '@codemirror/view'

/** The LaTeX logo: the raised A and the dropped E. */
export class LaTeXWidget extends WidgetType {
  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-tex')
    element.append('L', raised('a'), 'T', lowered('e'), 'X')
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

function raised(text: string) {
  const element = document.createElement('sup')
  element.textContent = text
  return element
}

function lowered(text: string) {
  const element = document.createElement('sub')
  element.textContent = text
  return element
}
