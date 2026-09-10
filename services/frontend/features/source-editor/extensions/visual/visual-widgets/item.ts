import { WidgetType } from '@codemirror/view'
import { ListEnvironmentName } from '../../../utils/tree-operations/ancestors'

/**
 * The bullet or number in front of an `\item`.
 *
 * Drawn by CSS counters rather than written into the DOM, because the marker
 * has to keep up as items are added and removed above it; the widget only
 * supplies the four custom properties the stylesheet counts with.
 */
export class ItemWidget extends WidgetType {
  public listType: string
  public suffix: string

  bullets: string[] = ['disc', 'circle', 'square']
  numbers: string[] = ['decimal', 'lower-alpha', 'lower-roman', 'upper-alpha']

  constructor(
    public currentEnvironment: ListEnvironmentName | 'document',
    public ordinal: number,
    public listDepth: number
  ) {
    super()

    if (currentEnvironment === 'itemize') {
      // unordered list
      this.listType = this.bullets[(listDepth - 1) % this.bullets.length]!
      this.suffix = "' '"
    } else {
      // ordered list
      this.listType = this.numbers[(listDepth - 1) % this.numbers.length]!
      this.suffix = "'. '"
    }
  }

  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-item')
    element.textContent = ' ' // a space, so the line has width
    this.setProperties(element)
    return element
  }

  eq(widget: ItemWidget) {
    return (
      widget.currentEnvironment === this.currentEnvironment &&
      widget.ordinal === this.ordinal &&
      widget.listDepth === this.listDepth
    )
  }

  updateDOM(element: HTMLElement) {
    this.setProperties(element)
    return true
  }

  ignoreEvent(event: Event): boolean {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }

  setProperties(element: HTMLElement) {
    element.style.setProperty('--list-depth', String(this.listDepth))
    element.style.setProperty('--list-ordinal', String(this.ordinal))
    element.style.setProperty('--list-type', this.listType)
    element.style.setProperty('--list-suffix', this.suffix)
  }
}
