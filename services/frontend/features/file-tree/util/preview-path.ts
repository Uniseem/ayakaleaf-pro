import type { FileEntry } from '@/lib/editor'

/** Where a graphic lives, and what kind of graphic it is. */
export type PreviewPath = {
  url: string
  extension: string
}

// `\includegraphics{plot}` names a file the compiler finds by trying
// extensions, and so does this. The order is the order the compiler prefers.
const EXTENSIONS = [
  '',
  '.png',
  '.jpg',
  '.jpeg',
  '.pdf',
  '.svg',
  '.PNG',
  '.JPG',
  '.JPEG',
  '.PDF',
  '.SVG',
]

/**
 * Resolves the path in an `\includegraphics` to something the browser can load.
 *
 * Addressed by the file's content hash where there is one, so a re-uploaded
 * image under the same name is a different URL and the browser cannot show the
 * old one from its cache.
 */
export function previewByPath(
  entryByPath: (path: string) => FileEntry | undefined,
  projectId: string,
  path: string
): PreviewPath | null {
  for (const suffix of EXTENSIONS) {
    const entry = entryByPath(path + suffix)

    if (entry && entry.kind === 'file') {
      const { name, id, hash } = entry
      const extension = name.slice(name.lastIndexOf('.') + 1)
      const url = hash
        ? `/api/projects/${projectId}/files/${id}?hash=${hash}`
        : `/api/projects/${projectId}/files/${id}`
      return { url, extension }
    }
  }
  return null
}
