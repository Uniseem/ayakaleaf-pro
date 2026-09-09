'use client'

import { memo, useCallback, useEffect, useRef } from 'react'
import { useCodeMirrorViewContext } from './codemirror-context'
import useCodeMirrorScope from '../hooks/use-codemirror-scope'

function CodeMirrorView() {
  const view = useCodeMirrorViewContext()

  // append the editor view dom to the container node when mounted
  const containerRef = useCallback(
    (node: HTMLDivElement | null) => {
      if (node) {
        node.appendChild(view.dom)
      }
    },
    [view]
  )

  // Destroy the editor when unmounted. The destroy is deferred a tick so
  // that React's development double-mount, which runs this cleanup and then
  // the effect again, does not leave the remounted component with a dead
  // view: the re-run cancels the pending destroy.
  const destroyTimer = useRef<number | null>(null)
  useEffect(() => {
    if (destroyTimer.current !== null) {
      window.clearTimeout(destroyTimer.current)
      destroyTimer.current = null
    }
    return () => {
      destroyTimer.current = window.setTimeout(() => {
        destroyTimer.current = null
        view.destroy()
      }, 0)
    }
  }, [view])

  useCodeMirrorScope(view)

  return <div ref={containerRef} style={{ height: '100%' }} />
}

export default memo(CodeMirrorView)
