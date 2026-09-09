import { useEffect } from 'react'
import { useCodeMirrorViewContext } from '../components/codemirror-context'
import { insertFigure } from '../extensions/toolbar/commands'

/**
 * Until the figure modal is ported, asking for it inserts the figure
 * environment the modal would have produced, with the cursor in the path.
 */
export function useFigureModalFallback() {
  const view = useCodeMirrorViewContext()

  useEffect(() => {
    const listener = () => {
      insertFigure(view)
      view.focus()
    }
    window.addEventListener('figure-modal:open', listener)
    return () => window.removeEventListener('figure-modal:open', listener)
  }, [view])
}
