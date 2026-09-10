/**
 * What can be pasted or dropped into the editor as a figure.
 *
 * The set is deliberately narrow: these are the formats the compiler will
 * accept from `\includegraphics` without further packages, so a file that
 * arrives here is one the document could actually use.
 */
export const ALLOWED_IMAGE_TYPES = new Set([
  'image/jpeg',
  'image/png',
  'application/pdf',
])

export function isAllowedImageType(mimeType: string): boolean {
  return ALLOWED_IMAGE_TYPES.has(mimeType)
}

export type PastedImageData = {
  name: string
  type: string
  data: Blob
}

/**
 * Hands a pasted image to the figure modal.
 *
 * Through a window event rather than a call, because the paste is noticed by a
 * CodeMirror handler and the modal is a React component somewhere else in the
 * tree; neither holds a reference to the other.
 */
export function dispatchFigureModalPasteEvent(imageData: PastedImageData): void {
  window.dispatchEvent(
    new CustomEvent<PastedImageData>('figure-modal:paste-image', {
      detail: imageData,
    })
  )
}

export async function findImageInClipboard(): Promise<File | null> {
  try {
    const clipboardItems = await navigator.clipboard.read()

    for (const item of clipboardItems) {
      for (const type of item.types) {
        if (isAllowedImageType(type)) {
          const blob = await item.getType(type)
          const file = new File([blob], `image.${type.split('/')[1]}`, { type })
          return file
        }
      }
    }
  } catch {
    // Clipboard.read() may fail in some browsers
  }

  return null
}

export const handleImagePaste = async (): Promise<boolean> => {
  const imageFile = await findImageInClipboard()
  if (imageFile) {
    dispatchFigureModalPasteEvent({
      name: imageFile.name,
      type: imageFile.type,
      data: imageFile,
    })
    return true
  }
  return false
}
