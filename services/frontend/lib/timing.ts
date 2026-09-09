/** Rate limiting for callbacks, with the shape lodash gives them. */

export type Debounced<T extends (...args: any[]) => unknown> = ((...args: Parameters<T>) => void) & {
  cancel: () => void
  flush: () => void
}

/** Calls `fn` once `wait` ms have passed without another call. */
export function debounce<T extends (...args: any[]) => unknown>(fn: T, wait: number): Debounced<T> {
  let timer: number | null = null
  let lastArgs: Parameters<T> | null = null

  const debounced = ((...args: Parameters<T>) => {
    lastArgs = args
    if (timer !== null) {
      window.clearTimeout(timer)
    }
    timer = window.setTimeout(() => {
      timer = null
      const callArgs = lastArgs
      lastArgs = null
      if (callArgs) {
        fn(...callArgs)
      }
    }, wait)
  }) as Debounced<T>

  debounced.cancel = () => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
    lastArgs = null
  }

  debounced.flush = () => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
      const callArgs = lastArgs
      lastArgs = null
      if (callArgs) {
        fn(...callArgs)
      }
    }
  }

  return debounced
}

/**
 * Calls `fn` at most once per `wait` ms. The first call runs at once; the
 * last call inside a window runs when the window ends (a trailing call).
 */
export function throttle<T extends (...args: any[]) => unknown>(
  fn: T,
  wait: number,
  options: { leading?: boolean; trailing?: boolean } = {}
): Debounced<T> {
  const leading = options.leading ?? true
  const trailing = options.trailing ?? true
  let last = 0
  let timer: number | null = null
  let lastArgs: Parameters<T> | null = null

  const run = (args: Parameters<T>) => {
    last = Date.now()
    fn(...args)
  }

  const throttled = ((...args: Parameters<T>) => {
    const now = Date.now()
    if (last === 0 && !leading) {
      last = now
    }
    const remaining = wait - (now - last)
    if (remaining <= 0 || remaining > wait) {
      if (timer !== null) {
        window.clearTimeout(timer)
        timer = null
      }
      run(args)
    } else if (trailing) {
      lastArgs = args
      if (timer === null) {
        timer = window.setTimeout(() => {
          timer = null
          const callArgs = lastArgs
          lastArgs = null
          if (callArgs) {
            run(callArgs)
          }
        }, remaining)
      }
    }
  }) as Debounced<T>

  throttled.cancel = () => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
    lastArgs = null
    last = 0
  }

  throttled.flush = () => {
    if (timer !== null && lastArgs) {
      window.clearTimeout(timer)
      timer = null
      const callArgs = lastArgs
      lastArgs = null
      run(callArgs)
    }
  }

  return throttled
}

export function round(value: number, precision = 0): number {
  const factor = 10 ** precision
  return Math.round(value * factor) / factor
}
