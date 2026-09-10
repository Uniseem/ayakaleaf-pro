import { WidgetType } from '@codemirror/view'

/**
 * A brace shown in place of one that has been hidden.
 *
 * Empty by default: an argument's braces normally vanish entirely, and the
 * widget exists so there is something at that position for the cursor to be
 * placed against. It carries content when the braces are being shown because
 * the cursor is inside them.
 */
export class BraceWidget extends WidgetType {
  constructor(private content = '') {
    super()
  }

  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-brace')
    element.textContent = this.content
    return element
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  eq(widget: BraceWidget) {
    return widget.content === this.content
  }

  updateDOM(element: HTMLSpanElement): boolean {
    element.textContent = this.content
    return true
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
