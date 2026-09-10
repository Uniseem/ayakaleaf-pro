import { WidgetType } from '@codemirror/view'

type NoteType = 'footnote' | 'endnote'

const symbols: Record<NoteType, string> = {
  footnote: '*',
  endnote: '†',
}

/**
 * A collapsed footnote, shown as the mark it prints as.
 *
 * The note's text is hidden behind it rather than deleted; clicking expands it
 * again, which is why this answers mouse events instead of ignoring them.
 */
export class FootnoteWidget extends WidgetType {
  constructor(private type: NoteType = 'footnote') {
    super()
  }

  toDOM() {
    const element = document.createElement('span')
    element.classList.add('ol-cm-footnote')
    element.setAttribute('role', 'button')
    element.textContent = symbols[this.type]
    return element
  }

  eq(widget: FootnoteWidget) {
    return this.type === widget.type
  }

  updateDOM(element: HTMLElement): boolean {
    element.textContent = symbols[this.type]
    return true
  }

  ignoreEvent(event: Event) {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
