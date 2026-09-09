/**
 * Console output that only shows when asked for.
 *
 * Set `localStorage.debugging = "true"` to see it. Everything else stays
 * quiet, so the console belongs to whoever is actually debugging.
 */
const enabled = (): boolean => {
  try {
    return typeof window !== 'undefined' && window.localStorage.getItem('debugging') === 'true'
  } catch {
    return false
  }
}

const when =
  (method: 'log' | 'debug' | 'warn' | 'error') =>
  (...args: unknown[]) => {
    if (enabled()) {
      console[method](...args)
    }
  }

export const debugConsole = {
  log: when('log'),
  debug: when('debug'),
  warn: when('warn'),
  error: when('error'),
}
