/**
 * The ways a name can be refused, as types rather than messages.
 *
 * The reason has to survive the trip from the check to the dialog that
 * explains it, and a message string would have to be matched on to be told
 * apart from another message string.
 */

export class InvalidFilenameError extends Error {
  constructor() {
    super('invalid filename')
    this.name = 'InvalidFilenameError'
  }
}

export class BlockedFilenameError extends Error {
  constructor() {
    super('blocked filename')
    this.name = 'BlockedFilenameError'
  }
}

export class DuplicateFilenameError extends Error {
  constructor() {
    super('duplicate filename')
    this.name = 'DuplicateFilenameError'
  }
}

export class DuplicateFilenameMoveError extends Error {
  constructor() {
    super('duplicate filename on move')
    this.name = 'DuplicateFilenameMoveError'
  }
}
