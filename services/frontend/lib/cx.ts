/** Joins class names, skipping anything falsy -- what classnames does. */
export type ClassValue = string | number | null | undefined | false | Record<string, unknown> | ClassValue[]

export function cx(...values: ClassValue[]): string {
  const names: string[] = []
  for (const value of values) {
    if (!value) {
      continue
    }
    if (typeof value === 'string' || typeof value === 'number') {
      names.push(String(value))
    } else if (Array.isArray(value)) {
      const inner = cx(...value)
      if (inner) {
        names.push(inner)
      }
    } else {
      for (const [name, on] of Object.entries(value)) {
        if (on) {
          names.push(name)
        }
      }
    }
  }
  return names.join(' ')
}

export default cx
