/**
 * localStorage holding JSON, and never throwing.
 *
 * Storage can be missing (a private window, a locked-down browser) or full,
 * and neither is worth breaking the editor over: a value that could not be
 * saved just is not remembered.
 */
const customLocalStorage = {
  getItem(key: string): any {
    try {
      const value = window.localStorage.getItem(key)
      return value === null ? null : JSON.parse(value)
    } catch {
      return null
    }
  },

  setItem(key: string, value: unknown) {
    try {
      window.localStorage.setItem(key, JSON.stringify(value))
    } catch {
      // storage is unavailable or full
    }
  },

  removeItem(key: string) {
    try {
      window.localStorage.removeItem(key)
    } catch {
      // storage is unavailable
    }
  },
}

export default customLocalStorage
