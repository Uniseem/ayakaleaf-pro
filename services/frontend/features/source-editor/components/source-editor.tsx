'use client'

import { lazy, memo, Suspense } from 'react'
import { FullSizeLoadingSpinner } from '@/components/ol/spinner'

const CodeMirrorEditor = lazy(() => import('./codemirror-editor'))

function SourceEditor() {
  return (
    <Suspense fallback={<FullSizeLoadingSpinner delay={500} />}>
      <CodeMirrorEditor />
    </Suspense>
  )
}

export default memo(SourceEditor)
