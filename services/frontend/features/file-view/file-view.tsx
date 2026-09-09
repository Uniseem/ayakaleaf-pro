'use client'

/**
 * A file that is not text.
 *
 * An image gets shown, a PDF gets the browser's viewer, and anything else gets
 * its size and a way to download it -- which is more honest than opening a
 * binary in a text editor and showing whatever its bytes happen to look like.
 */

import { Button } from '@heroui/react'
import { useMemo } from 'react'
import type { FileEntry } from '@/lib/editor'
import { extensionOf } from '@/features/file-tree/tree'
import { useProject } from '@/features/ide/contexts/project-context'

// What the API will serve inline. SVG is deliberately absent: it is a
// document that can carry script, so the API sends it as bytes to save rather
// than as something the browser renders on this origin.
const IMAGES = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'avif', 'bmp', 'ico'])

function readableSize(bytes?: number): string {
  if (bytes === undefined) {
    return 'unknown size'
  }
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} kB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function FileView({
  entry,
  onClose,
}: {
  entry: FileEntry
  onClose: () => void
}) {
  const { projectId } = useProject()
  const extension = extensionOf(entry.name)

  // Served by content hash, so the browser can cache it and a changed file
  // gets a different address rather than a stale one.
  const source = useMemo(
    () =>
      entry.hash
        ? `/api/projects/${projectId}/files/${entry.id}?hash=${entry.hash}`
        : `/api/projects/${projectId}/files/${entry.id}`,
    [projectId, entry.id, entry.hash]
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-default-50">
      <header className="flex items-center gap-2 border-b border-divider bg-background px-3 py-2">
        <span className="truncate text-sm font-medium" title={entry.path}>
          {entry.name}
        </span>
        <span className="text-xs text-default-400">{readableSize(entry.size)}</span>
        <div className="flex-1" />
        <Button
          as="a"
          href={source}
          download={entry.name}
          size="sm"
          variant="flat"
          className="h-7"
        >
          Download
        </Button>
        <Button size="sm" variant="light" className="h-7" onPress={onClose}>
          Close
        </Button>
      </header>

      <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4">
        {IMAGES.has(extension) ? (
          <img
            src={source}
            alt={entry.name}
            className="max-h-full max-w-full object-contain"
          />
        ) : extension === 'pdf' ? (
          <iframe src={source} title={entry.name} className="h-full w-full border-0" />
        ) : (
          <div className="text-center">
            <p className="text-sm text-default-600">
              {extension === 'svg'
                ? 'SVG files are downloaded rather than shown, because they can carry script.'
                : 'This file cannot be shown here.'}
            </p>
            <p className="mt-1 text-xs text-default-400">
              {extension ? `.${extension}` : 'No extension'} ·{' '}
              {readableSize(entry.size)}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
