'use client'

import { memo } from 'react'
import PdfPreviewPane from './pdf-preview-pane'
import { withErrorBoundary } from '@/components/ol/error-boundary'
import PdfPreviewErrorBoundaryFallback from './pdf-preview-error-boundary-fallback'
import { useLayout } from '@/features/ide/contexts/layout-context'

function PdfPreview() {
  const { detachRole } = useLayout()
  if (detachRole === 'detacher') return null
  return <PdfPreviewPane />
}

export default withErrorBoundary(memo(PdfPreview), () => <PdfPreviewErrorBoundaryFallback type="preview" />)
