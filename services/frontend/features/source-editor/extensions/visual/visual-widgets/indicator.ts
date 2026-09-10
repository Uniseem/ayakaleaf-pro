import { WidgetType } from '@codemirror/view'

/** A short piece of text standing in for markup, such as a footnote's marker. */
export class IndicatorWidget extends WidgetType {
  constructor(public content: string) {
    super()
  }

  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-indicator')
    element.textContent = this.content
    return element
  }

  eq(widget: IndicatorWidget) {
    return widget.content === this.content
  }

  updateDOM(element: HTMLElement): boolean {
    element.textContent = this.content
    return true
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
