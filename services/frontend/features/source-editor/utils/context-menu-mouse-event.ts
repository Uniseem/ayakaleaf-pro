import { isMac } from '@/lib/os'

/** Whether a mouse event is the one that opens a context menu on this platform. */
export function isContextMenuMouseEvent(event: MouseEvent): boolean {
  return event.button === 2 || (isMac && event.button === 0 && event.ctrlKey)
}
