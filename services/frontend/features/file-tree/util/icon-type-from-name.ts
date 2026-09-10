import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'

/**
 * The icon for a file, chosen by extension.
 *
 * From file-tree/util/icon-type-from-name. What matters at this size is what
 * opening it will do, not what made it: an image opens in the viewer, a
 * bibliography is neither text nor picture, and everything else is a document.
 */
export const iconTypeFromName = (name: string): AvailableUnfilledIcon => {
  let ext = name.split('.').pop()
  ext = ext ? ext.toLowerCase() : ext

  if (ext && ['png', 'pdf', 'jpg', 'jpeg', 'gif'].includes(ext)) {
    return 'image'
  } else if (ext && ['csv', 'xls', 'xlsx'].includes(ext)) {
    return 'table_chart'
  } else if (ext && ['py', 'r'].includes(ext)) {
    return 'code'
  } else if (ext && ['bib'].includes(ext)) {
    return 'book_5'
  }
  return 'description'
}

export default iconTypeFromName
