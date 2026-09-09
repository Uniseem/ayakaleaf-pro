'use client'

/**
 * The hooks the editor's panels share.
 *
 * Each one exists because the same three lines were about to be written in
 * four places: a preference that must outlive a reload, a listener that must
 * be removed, a size that must be watched.
 */

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from 'react'

/**
 * State that survives a reload.
 *
 * The first render must not read storage: the server renders too, and a value
 * only the browser has would make the two disagree and React would throw the
 * markup away. So it starts at the default and is corrected on mount, which
 * is one frame of the default and then the real value.
 */
export function usePersistedState<T>(
  key: string,
  fallback: T
): [T, (value: T | ((previous: T) => T)) => void] {
  const [value, setValue] = useState<T>(fallback)

  useEffect(() => {
    try {
      const stored = window.localStorage.getItem(key)
      if (stored !== null) {
        setValue(JSON.parse(stored) as T)
      }
    } catch {
      // Private windows, cleared site data, a browser told to block storage:
      // all of them mean the default, none of them mean an error.
    }
  }, [key])

  const write = useCallback(
    (next: T | ((previous: T) => T)) => {
      setValue(previous => {
        const resolved =
          typeof next === 'function' ? (next as (p: T) => T)(previous) : next
        try {
          window.localStorage.setItem(key, JSON.stringify(resolved))
        } catch {
          // Not being able to remember it is not a reason to refuse to do it.
        }
        return resolved
      })
    },
    [key]
  )

  return [value, write]
}

/** An event listener that is removed when it should be. */
export function useEventListener<K extends keyof WindowEventMap>(
  type: K,
  listener: (event: WindowEventMap[K]) => void,
  target?: RefObject<HTMLElement | null>
) {
  // Kept in a ref so that changing the handler does not detach and reattach
  // the listener, which would drop events in between.
  const held = useRef(listener)
  useLayoutEffect(() => {
    held.current = listener
  }, [listener])

  useEffect(() => {
    const element = target ? target.current : window
    if (!element) {
      return
    }
    const handler = (event: Event) => held.current(event as WindowEventMap[K])
    element.addEventListener(type, handler)
    return () => element.removeEventListener(type, handler)
  }, [type, target])
}

/** The size of an element, as it changes. */
export function useResizeObserver(ref: RefObject<HTMLElement | null>) {
  const [size, setSize] = useState<{ width: number; height: number }>({
    width: 0,
    height: 0,
  })

  useEffect(() => {
    const element = ref.current
    if (!element || typeof ResizeObserver === 'undefined') {
      return
    }
    const observer = new ResizeObserver(entries => {
      const rect = entries[0]?.contentRect
      if (rect) {
        setSize({ width: rect.width, height: rect.height })
      }
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])

  return size
}

/** Whether the component is still mounted, for work that finishes late. */
export function useIsMounted() {
  const mounted = useRef(false)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return useCallback(() => mounted.current, [])
}

/** A value from the render before this one. */
export function usePrevious<T>(value: T): T | undefined {
  const previous = useRef<T | undefined>(undefined)
  useEffect(() => {
    previous.current = value
  }, [value])
  return previous.current
}

/** A debounced copy of a value. */
export function useDebounced<T>(value: T, delay: number): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return settled
}

/** An AbortController that aborts when the component goes away. */
export function useAbortController() {
  const ref = useRef<AbortController>(new AbortController())
  useEffect(() => {
    const controller = ref.current
    return () => controller.abort()
  }, [])
  return ref.current
}

/** Whether this is a Mac, which decides what the modifier key is called. */
export function isMac(): boolean {
  return (
    typeof navigator !== 'undefined' && /Mac|iPod|iPhone|iPad/.test(navigator.platform)
  )
}
