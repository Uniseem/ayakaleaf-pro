import { EditorView } from '@codemirror/view'
import { EditorSelection, EditorState, StateCommand } from '@codemirror/state'
import { SearchQuery, selectWord } from '@codemirror/search'

export { selectNextOccurrence } from '@codemirror/search'

const findPrevOccurence = (state: EditorState, search: string) => {
  const searchQuery = new SearchQuery({ search, literal: true })
  const { from, to } = state.selection.main
  // walk the matches, keeping the last one that ends before the selection
  const cursor = searchQuery.getCursor(state.doc)
  let previous: { from: number; to: number } | null = null
  for (let result = cursor.next(); !result.done; result = cursor.next()) {
    if (result.value.to <= from) {
      previous = result.value
    } else {
      break
    }
  }
  if (previous) {
    return previous
  }
  // wrap around to the last match after the selection
  let last: { from: number; to: number } | null = null
  const wrapCursor = searchQuery.getCursor(state.doc, to)
  for (let result = wrapCursor.next(); !result.done; result = wrapCursor.next()) {
    last = result.value
  }
  return last
}

export const selectPrevOccurrence: StateCommand = ({ state, dispatch }) => {
  const { ranges } = state.selection

  if (ranges.some(range => range.from === range.to)) {
    return selectWord({ state, dispatch })
  }

  const searchedText = state.sliceDoc(ranges[0]!.from, ranges[0]!.to)

  if (state.selection.ranges.some(range => state.sliceDoc(range.from, range.to) !== searchedText)) {
    return false
  }

  const range = findPrevOccurence(state, searchedText)
  if (!range) {
    return false
  }

  dispatch(
    state.update({
      selection: state.selection.addRange(EditorSelection.range(range.from, range.to)),
      effects: EditorView.scrollIntoView(range.to),
    })
  )

  return true
}
