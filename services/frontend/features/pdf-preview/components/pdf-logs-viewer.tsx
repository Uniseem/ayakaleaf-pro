'use client'

import cx from '@/lib/cx'
import { useCompile } from '@/features/ide/contexts/compile-context'
import ErrorLogs from './error-logs'
import { usePdfPreviewContext } from './pdf-preview-provider'

/** The logs, over the PDF, whenever they are asked for or the viewer failed. */
export default function PdfLogsViewer() {
  const { showLogs } = useCompile()
  const { loadingError } = usePdfPreviewContext()

  return (
    <div
      className={cx('new-logs-pane', {
        hidden: !showLogs && !loadingError,
      })}
    >
      <ErrorLogs includeActionButtons />
    </div>
  )
}
