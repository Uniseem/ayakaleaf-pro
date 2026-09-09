import type { Text } from '@codemirror/state'

/** A document position for a line and column, clamped into the document. */
export const findValidPosition = (
  doc: Text,
  lineNumber: number, // 1-indexed
  columnNumber = 0 // 0-indexed
): number => {
  if (lineNumber < 1) {
    return 0
  }

  const lines = doc.lines

  if (lineNumber > lines) {
    return doc.length
  }

  const line = doc.line(lineNumber)

  // the requested line and column, but not past the end of the line
  return Math.min(line.from + columnNumber, line.to)
}
