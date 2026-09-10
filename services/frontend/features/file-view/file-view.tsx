'use client'

/**
 * A file that is not text, from features/file-view.
 *
 * An image gets shown, a PDF gets the browser's viewer, and anything else gets
 * a way to download it -- which is more honest than opening a binary in a text
 * editor and showing whatever its bytes happen to look like.
 */

import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'
import { LoadingSpinner } from '@/components/ol/spinner'
import type { FileEntry } from '@/lib/editor'
import { extensionOf } from '@/features/file-tree/tree'
import { useProject } from '@/features/ide/contexts/project-context'

// What the API will serve inline. SVG is deliberately absent: it is a
// document that can carry script, so the API sends it as bytes to save rather
// than as something the browser renders on this origin.
const IMAGE_EXTENSIONS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'avif', 'bmp', 'ico'])

export function FileView({ entry }: { entry: FileEntry; onClose?: () => void }) {
  const { t } = useTranslation()
  const { projectId } = useProject()
  const [contentLoading, setContentLoading] = useState(true)
  const [hasError, setHasError] = useState(false)

  const extension = extensionOf(entry.name)

  const isImageFile = IMAGE_EXTENSIONS.has(extension)
  const isPdfFile = extension === 'pdf'
  const isUnpreviewableFile = !isImageFile && !isPdfFile

  // Served by content hash, so the browser can cache it and a changed file
  // gets a different address rather than a stale one.
  const source = useMemo(
    () =>
      entry.hash
        ? `/api/projects/${projectId}/files/${entry.id}?hash=${entry.hash}`
        : `/api/projects/${projectId}/files/${entry.id}`,
    [projectId, entry.id, entry.hash]
  )

  const handleLoad = useCallback(() => setContentLoading(false), [])

  const handleError = useCallback(() => {
    setContentLoading(false)
    setHasError(true)
  }, [])

  return (
    <div className="file-view full-size">
      {!hasError && (
        <>
          <div className="file-view-buttons">
            <Button variant="secondary" download={entry.name} href={source}>
              <MaterialIcon type="download" className="align-middle" /> <span>{t('download')}</span>
            </Button>
          </div>

          {isImageFile && (
            <img
              src={source}
              alt={entry.name}
              onLoad={handleLoad}
              onError={handleError}
              className="file-view-image"
            />
          )}

          {isPdfFile && (
            <iframe
              title={entry.name}
              src={source}
              onLoad={handleLoad}
              onError={handleError}
              className="file-view-pdf"
            />
          )}
        </>
      )}

      {!isUnpreviewableFile && contentLoading && !hasError && (
        <div className="loading-panel loading-panel-file-view" data-testid="loading-panel-file-view">
          <LoadingSpinner />
        </div>
      )}

      {(isUnpreviewableFile || hasError) && <p className="no-preview">{t('no_preview_available')}</p>}

      {/*
        Safari sends key events to the editor behind this view even when
        CodeMirror is not focused, which edits the document somebody is only
        looking at. Parking the focus here stops that.
      */}
      <textarea
        autoFocus
        aria-label="Invisible element to manage focus and prevent unintended behavior"
        tabIndex={-1}
        style={{ position: 'absolute', left: '-9999px' }}
      />
    </div>
  )
}

export default FileView
