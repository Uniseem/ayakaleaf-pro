'use client'

/**
 * A list that loads older items when scrolled to the top, from
 * chat/components/infinite-scroll.
 *
 * The distance from the bottom is what is kept steady, not the scroll offset:
 * prepending older messages changes every offset above them, and holding the
 * offset would make the list jump by the height of what just loaded.
 */

import { useCallback, useEffect, useLayoutEffect, useRef, type ReactNode } from 'react'
import { debounce } from '@/lib/timing'

const SCROLL_END_OFFSET = 30

export function InfiniteScroll({
  atEnd,
  children,
  className = '',
  fetchData,
  itemCount,
  isLoading,
}: {
  atEnd?: boolean
  children: ReactNode
  className?: string
  fetchData: () => void
  itemCount: number
  isLoading?: boolean
}) {
  const root = useRef<HTMLDivElement>(null)

  // In a ref rather than state, so the effects can read it without listing it.
  const scrollBottomRef = useRef(0)

  const updateScrollPosition = useCallback(() => {
    if (root.current) {
      root.current.scrollTop =
        root.current.scrollHeight - root.current.clientHeight - scrollBottomRef.current
    }
  }, [])

  // After new items arrive.
  useLayoutEffect(updateScrollPosition, [itemCount, updateScrollPosition])

  // And after the window changes size.
  useEffect(() => {
    const handleResize = debounce(updateScrollPosition, 400)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [updateScrollPosition])

  const shouldFetchData = useCallback(() => {
    const element = root.current
    if (!element) {
      return false
    }
    const firstChild = element.children[0] as HTMLElement | undefined
    const containerIsLargerThanContent = (firstChild?.clientHeight ?? 0) < element.clientHeight
    if (atEnd || isLoading || containerIsLargerThanContent) {
      return false
    }
    return element.scrollTop < SCROLL_END_OFFSET
  }, [atEnd, isLoading])

  const onScrollHandler = useCallback(
    (event: React.UIEvent<HTMLDivElement>) => {
      const element = root.current
      if (!element) {
        return
      }
      scrollBottomRef.current = element.scrollHeight - element.scrollTop - element.clientHeight

      // Scrolling inside a nested element is not this list scrolling.
      if (event.target !== event.currentTarget) {
        return
      }
      if (shouldFetchData()) {
        fetchData()
      }
    },
    [fetchData, shouldFetchData]
  )

  return (
    <div ref={root} onScroll={onScrollHandler} className={className}>
      {children}
    </div>
  )
}

export default InfiniteScroll
