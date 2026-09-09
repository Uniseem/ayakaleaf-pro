import { ChangeSet, EditorState, StateField } from '@codemirror/state'
import {
  ProjectionItem,
  ProjectionResult,
  getUpdatedProjection,
  EnterNodeFn,
  ProjectionStatus,
} from './tree-operations/projection'
import { languageLoadedEffect } from '../extensions/language'

export function mergeChangeRanges(changes: ChangeSet) {
  let fromA = Number.MAX_VALUE
  let fromB = Number.MAX_VALUE
  let toA = Number.MIN_VALUE
  let toB = Number.MIN_VALUE
  changes.iterChangedRanges((changeFromA, changeToA, changeFromB, changeToB) => {
    fromA = Math.min(changeFromA, fromA)
    fromB = Math.min(changeFromB, fromB)
    toA = Math.max(changeToA, toA)
    toB = Math.max(changeToB, toB)
  })
  return { fromA, toA, fromB, toB }
}

/**
 * Creates a StateField to manage a 'projection' of the document: the items
 * of type T that enterNode picks out of the syntax tree, kept up to date as
 * the document changes.
 */
export function makeProjectionStateField<T extends ProjectionItem>(enterNode: EnterNodeFn<T>): StateField<ProjectionResult<T>> {
  const initialiseProjection = (state: EditorState) =>
    getUpdatedProjection(state, 0, state.doc.length, 0, state.doc.length, true, enterNode)

  return StateField.define<ProjectionResult<T>>({
    create(state) {
      return initialiseProjection(state)
    },
    update(currentProjection, transaction) {
      if (transaction.effects.some(effect => effect.is(languageLoadedEffect))) {
        return initialiseProjection(transaction.state)
      }

      if (transaction.docChanged || currentProjection.status !== ProjectionStatus.Complete) {
        const { fromA, toA, fromB, toB } = mergeChangeRanges(transaction.changes)
        return getUpdatedProjection<T>(transaction.state, fromA, toA, fromB, toB, false, enterNode, transaction, currentProjection)
      }
      return currentProjection
    },
  })
}
