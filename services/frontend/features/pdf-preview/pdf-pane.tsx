'use client'

/**
 * The PDF pane, as the layout mounts it. The pane itself lives in
 * components/pdf-preview; this keeps the name the layout imports.
 */

import PdfPreview from './components/pdf-preview'

export function PdfPane() {
  return <PdfPreview />
}

export default PdfPane
