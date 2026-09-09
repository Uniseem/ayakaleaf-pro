import { StateEffect, StateField } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { updateHasEffect } from '../../utils/effects'

/**
 * Whether the mouse button is down inside the editor, which the visual
 * editor uses to keep decorations from jumping mid-drag.
 */
export const mouseDownEffect = StateEffect.define<boolean>()
export const updateHasMouseDownEffect = updateHasEffect(mouseDownEffect)

export const mouseDownState = StateField.define<boolean>({
  create() {
    return false
  },
  update(value, tr) {
    for (const effect of tr.effects) {
      if (effect.is(mouseDownEffect)) {
        value = effect.value
      }
    }
    return value
  },
})

export const mousedown = [
  mouseDownState,
  EditorView.domEventHandlers({
    mousedown(event, view) {
      view.dispatch({ effects: mouseDownEffect.of(true) })
      const listener = () => {
        window.removeEventListener('mouseup', listener)
        window.setTimeout(() => {
          view.dispatch({ effects: mouseDownEffect.of(false) })
        })
      }
      window.addEventListener('mouseup', listener)
    },
  }),
]
