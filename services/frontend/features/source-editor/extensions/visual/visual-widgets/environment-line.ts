import { WidgetType } from '@codemirror/view'

/**
 * The rule drawn where an environment begins or ends.
 *
 * Replaces the `\begin{...}` and `\end{...}` lines: the reader still needs to
 * see where the environment's extent is once its markup is hidden, and a rule
 * says that without taking a line of text to say it.
 */
export class EnvironmentLineWidget extends WidgetType {
  constructor(
    public environment: string,
    public line?: 'begin' | 'end'
  ) {
    super()
  }

  toDOM() {
    const element = document.createElement('div')
    element.classList.add(`ol-cm-environment-${this.environment}`)
    element.classList.add('ol-cm-environment-edge')

    const line = document.createElement('div')
    element.append(line)

    line.classList.add('ol-cm-environment-line')
    line.classList.add(`ol-cm-environment-${this.environment}`)
    switch (this.line) {
      case 'begin':
        element.classList.add('ol-cm-environment-top')
        line.classList.add('ol-cm-environment-first-line')
        break
      case 'end':
        element.classList.add('ol-cm-environment-bottom')
        line.classList.add('ol-cm-environment-last-line')
        break
    }

    return element
  }

  eq(widget: EnvironmentLineWidget) {
    return widget.environment === this.environment && widget.line === this.line
  }

  ignoreEvent(event: Event): boolean {
    return event.type !== 'mousedown' && event.type !== 'mouseup'
  }

  coordsAt(element: HTMLElement) {
    return element.getBoundingClientRect()
  }
}
