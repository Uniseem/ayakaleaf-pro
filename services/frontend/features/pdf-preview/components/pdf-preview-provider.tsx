'use client'

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

/**
 * What the pane's parts share among themselves but nothing outside needs:
 * whether pdf.js itself failed to start, which the logs pane shows in place
 * of the PDF.
 */
const PdfPreviewContext = createContext<
  | {
      loadingError: boolean
      setLoadingError: (value: boolean) => void
    }
  | undefined
>(undefined)

export const usePdfPreviewContext = () => {
  const context = useContext(PdfPreviewContext)
  if (!context) {
    throw new Error('usePdfPreviewContext is only available inside PdfPreviewProvider')
  }
  return context
}

export const PdfPreviewProvider = ({ children }: { children: ReactNode }) => {
  const [loadingError, setLoadingError] = useState(false)

  const value = useMemo(() => ({ loadingError, setLoadingError }), [loadingError])

  return <PdfPreviewContext.Provider value={value}>{children}</PdfPreviewContext.Provider>
}
