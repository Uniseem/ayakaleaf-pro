import { WidgetType } from '@codemirror/view'

/** A brace replaced by an icon, for commands drawn as a symbol. */
export class IconBraceWidget extends WidgetType {
  constructor(private content = '') {
    super()
  }

  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-brace')
    element.classList.add('ol-cm-icon-brace')
    element.textContent = this.content
    return element
  }

  ignoreEvent(event: Event): boolean {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  eq(widget: IconBraceWidget) {
    return widget.content === this.content
  }

  updateDOM(element: HTMLElement): boolean {
    element.textContent = this.content
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
