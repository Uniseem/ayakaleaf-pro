/**
 * Whether a name may be given to a file.
 *
 * The same rules as the original's file-tree/util/safe-path, which are also
 * the server's: the client checks first so that a bad name is refused while
 * it is still being typed, and the server checks again because a check that
 * only runs in a browser is not a check.
 */

// Separators, the wildcard, and every control or surrogate code point. A name
// carrying a slash would silently mean a different folder.
const BADCHAR_RX = new RegExp('[/\\\\*\\u0000-\\u001F\\u007F\\u0080-\\u009F\\uD800-\\uDFFF]', 'g')

// "." and ".." are directories, not names, and leading or trailing whitespace
// makes two files that look identical.
const BADFILE_RX = new RegExp('(^\\.$)|(^\\.\\.$)|(^\\s+)|(\\s+$)', 'g')

// Names that collide with the properties every JavaScript object already has.
// Somewhere in the stack a set of files becomes a plain object keyed by name,
// and a file called "constructor" turns that lookup into something else.
const BLOCKEDFILE_RX =
  /^(prototype|constructor|toString|toLocaleString|valueOf|hasOwnProperty|isPrototypeOf|propertyIsEnumerable|__defineGetter__|__lookupGetter__|__defineSetter__|__lookupSetter__|__proto__)$/

const MAX_PATH = 1024

export function isCleanFilename(filename: string): boolean {
  return isAllowedLength(filename) && !filename.match(BADCHAR_RX) && !filename.match(BADFILE_RX)
}

export function isBlockedFilename(filename: string): boolean {
  return BLOCKEDFILE_RX.test(filename)
}

export function isAllowedLength(pathname: string): boolean {
  return pathname.length > 0 && pathname.length <= MAX_PATH
}
