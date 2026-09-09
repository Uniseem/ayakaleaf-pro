'use client'

import type { ReactNode } from 'react'

/** Where notices float over the top of the PDF. */
export const PdfPreviewMessages = ({ children }: { children?: ReactNode }) => {
  return <div className="pdf-preview-messages">{children}</div>
}
