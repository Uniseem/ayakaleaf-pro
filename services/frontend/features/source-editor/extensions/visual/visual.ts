import { Compartment, EditorState, Extension, StateEffect, StateField, TransactionSpec } from '@codemirror/state'
import { EditorView, ViewPlugin } from '@codemirror/view'
import { forceParsing, syntaxTree } from '@codemirror/language'
import { hasLanguageLoadedEffect } from '../language'
import { restoreScrollPosition } from '../scroll-position'
import { mousedown, mouseDownEffect } from './selection'
import { visualMode } from '../../visual'
import { debugConsole } from '@/lib/debug'

type Options = {
  visual: boolean
}

const visualConf = new Compartment()

export const toggleVisualEffect = StateEffect.define<boolean>()

const visualState = StateField.define<boolean>({
  create() {
    return false
  },
  update(value, tr) {
    for (const effect of tr.effects) {
      if (effect.is(toggleVisualEffect)) {
        return effect.value
      }
    }
    return value
  },
})

const configureVisualExtensions = (options: Options) => (options.visual ? sharedVisualExtensions() : [])

export const visual = (docName: string, options: Options): Extension => {
  return [
    visualState.init(() => options.visual),
    visualConf.of(configureVisualExtensions(options)),
    visualOnly(options.visual, latexVisualExtensions()),
  ]
}

export const isVisual = (view: EditorView) => {
  return view.state.field(visualState, false) || false
}

export const setVisual = (options: Options): TransactionSpec => {
  return {
    effects: [toggleVisualEffect.of(options.visual), visualConf.reconfigure(configureVisualExtensions(options))],
  }
}

// Loads `extension` only while the editor is in a particular mode, reacting to
// mode switches via `toggleVisualEffect`. `activeWhenVisual` selects which mode:
// `true` for visual-only, `false` for source-only.
const modeOnly = (activeWhenVisual: boolean, visual: boolean, extension: Extension) => {
  const conf = new Compartment()
  const configure = (visual: boolean) => (visual === activeWhenVisual ? extension : [])
  return [
    conf.of(configure(visual)),

    // Respond to switching editor modes
    EditorState.transactionExtender.of(tr => {
      for (const effect of tr.effects) {
        if (effect.is(toggleVisualEffect)) {
          return {
            effects: conf.reconfigure(configure(effect.value)),
          }
        }
      }
      return null
    }),
  ]
}

export const visualOnly = (visual: boolean, extension: Extension) => modeOnly(true, visual, extension)

export const sourceOnly = (visual: boolean, extension: Extension) => modeOnly(false, visual, extension)

const parsedAttributesConf = new Compartment()

/**
 * A view plugin which shows the editor content, makes it focusable,
 * and restores the scroll position, once the initial decorations have been applied.
 */
export const showContentWhenParsed = [
  parsedAttributesConf.of([EditorView.editable.of(false)]),
  ViewPlugin.define(view => {
    const showContent = () => {
      view.dispatch(
        {
          effects: parsedAttributesConf.reconfigure([
            EditorView.editorAttributes.of({
              class: 'ol-cm-parsed',
            }),
            EditorView.editable.of(true),
          ]),
        },
        restoreScrollPosition()
      )
      view.focus()
    }

    // already parsed
    if (syntaxTree(view.state).length === view.state.doc.length) {
      window.setTimeout(showContent)
      return {}
    }

    // as a fallback, make sure the content is visible after 5s
    const fallbackTimer = window.setTimeout(showContent, 5000)

    let languageLoaded = false

    return {
      update(update) {
        // wait for the language to load before telling the parser to run
        if (!languageLoaded && hasLanguageLoadedEffect(update)) {
          languageLoaded = true
          // in a timeout, as this is already in a dispatch cycle
          window.setTimeout(() => {
            // run asynchronously
            new Promise(() => {
              // tell the parser to run until the end of the document
              forceParsing(view, view.state.doc.length, Infinity)
              // clear the fallback timeout
              window.clearTimeout(fallbackTimer)
              // show the content, in a timeout so the decorations can build first
              window.setTimeout(showContent)
            }).catch(debugConsole.error)
          })
        }
      },
    }
  }),
]

/**
 * A transaction extender which scrolls mouse clicks into view, in case decorations have moved the cursor out of view.
 */
const scrollJumpAdjuster = EditorState.transactionExtender.of(tr => {
  // Attach a "scrollIntoView" effect on all mouse selections to adjust for
  // any jumps that may occur when hiding/showing decorations.
  if (!tr.scrollIntoView) {
    for (const effect of tr.effects) {
      if (effect.is(mouseDownEffect) && effect.value === false) {
        return {
          effects: EditorView.scrollIntoView(tr.newSelection.main.head),
        }
      }
    }
  }

  return {}
})

const sharedVisualExtensions = () => [
  mousedown,
  scrollJumpAdjuster,
  showContentWhenParsed,
  EditorView.contentAttributes.of({ 'aria-label': 'Visual Editor editing' }),
]

/**
 * What the visual editor draws. The full set of decorations from the
 * original (atomic and mark decorations, the list marker, paste handling,
 * the table generator) is its own phase; what is here is the visual mode
 * the new stack already had.
 */
const latexVisualExtensions = (): Extension => [visualMode()]
