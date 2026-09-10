import { EditorView, rectangularSelection, tooltips, crosshairCursor, dropCursor, highlightActiveLineGutter } from '@codemirror/view'
import { EditorSelection, EditorState, Extension } from '@codemirror/state'
import { foldGutter, indentOnInput, indentUnit } from '@codemirror/language'
import { history } from '@codemirror/commands'
import type { SearchQuery } from '@codemirror/search'
import { language, type Metadata } from './language'
import { lineWrappingIndentation } from './line-wrapping-indentation'
import { theme, type ActiveOverallTheme } from './theme'
import { realtime } from './realtime'
import { cursorPosition } from './cursor-position'
import { scrollPosition } from './scroll-position'
import { annotations } from './annotations'
import { cursorHighlights } from './cursor-highlights'
import { autoComplete } from './auto-complete'
import { editable } from './editable'
import { autoPair } from './auto-pair'
import { phrases } from './phrases'
import { mathPreview } from './math-preview'
import { reviewTooltip } from './review-tooltip'
import { spelling } from './spelling'
import type { HunspellManager } from '../hunspell/HunspellManager'
import type { PreviewPath } from '@/features/file-tree/util/preview-path'
import { symbolPalette } from './symbol-palette'
import { search } from './search'
import { filterCharacters } from './filter-characters'
import { keybindings } from './keybindings'
import { bracketMatching, bracketSelection } from './bracket-matching'
import { verticalOverflow } from './vertical-overflow'
import { lineNumbers } from './line-numbers'
import { highlightActiveLine } from './highlight-active-line'
import { emptyLineFiller } from './empty-line-filler'
import { goToLinePanel } from './go-to-line'
import { drawSelection } from './draw-selection'
import { nonBlinkingCursor } from './non-blinking-cursor'
import { sourceOnly, visual } from './visual/visual'
import { inlineBackground } from './inline-background'
import { indentationMarkers } from './indentation-markers'
import { keymaps } from './keymaps'
import { shortcuts } from './shortcuts'
import { effectListeners } from './effect-listeners'
import { highlightSpecialChars } from './highlight-special-chars'
import { toolbarPanel } from './toolbar/toolbar-panel'
import { breadcrumbPanel } from './breadcrumbs-panel'
import { geometryChangeEvent } from './geometry-change-event'
import { docName } from './doc-name'
import { fileTreeItemDrop } from './file-tree-item-drop'
import { tooltipsReposition } from './tooltips-reposition'
import { selectionListener } from './selection-listener'
import { tabsListener } from './tabs-listener'
import { trackedChanges } from '../tracked-changes'
import type { FontFamily, LineHeight } from '../utils/styles'

export type ExtensionOptions = {
  currentDoc: {
    doc_id: string
  }
  docName: string
  theme: {
    fontFamily: FontFamily
    fontSize: number
    lineHeight: LineHeight
    activeOverallTheme: ActiveOverallTheme
    editorTheme: string
  }
  metadata: Metadata
  settings: {
    autoComplete: boolean
    autoPairDelimiters: boolean
    mode: string
    syntaxValidation: boolean
    mathPreview: boolean
    editorTabs: boolean
    nonBlinkingCursor: boolean
  }
  phrases: Record<string, string>
  spelling: {
    spellCheckLanguage?: string
    hunspellManager?: HunspellManager
  }
  visual: {
    visual: boolean
    previewByPath: (path: string) => PreviewPath | null
  }
  initialSearchQuery: SearchQuery | null
  handleException: (exception: unknown) => void
  onLocalChange: (text: string) => void
  setEditorSelection: (selection: EditorSelection | undefined) => void
}

export const createExtensions = (options: ExtensionOptions): Extension[] => [
  lineNumbers(),
  highlightSpecialChars(options.visual.visual),
  // The built-in extension that manages the history stack,
  // configured to increase the maximum delay between adjacent grouped edits
  history({ newGroupDelay: 250 }),
  // The built-in extension that displays buttons for folding code in a gutter element,
  // configured with custom openText and closeText symbols.
  foldGutter({
    openText: '▾',
    closedText: '▸',
  }),
  drawSelection(),
  nonBlinkingCursor(),
  // A built-in facet that is set to true to allow multiple selections.
  // This makes the editor more like a code editor than Google Docs or Microsoft Word,
  // which only have single selections.
  EditorState.allowMultipleSelections.of(true),
  // A built-in extension that enables soft line wrapping.
  EditorView.lineWrapping,
  sourceOnly(options.visual.visual, EditorView.contentAttributes.of({ 'aria-label': 'Source Editor editing' })),
  // A built-in extension that re-indents input if the language defines an indentOnInput field in its language data.
  indentOnInput(),
  lineWrappingIndentation(options.visual.visual),
  indentationMarkers(options.visual.visual),
  bracketMatching(),
  bracketSelection(),
  // A built-in extension that enables rectangular selections, created by dragging a new selection while holding down Alt.
  rectangularSelection(),
  // A built-in extension that turns the pointer into a crosshair while Alt is pressed.
  crosshairCursor(),
  // A built-in extension that shows where dragged content will be dropped.
  dropCursor(),
  // A built-in extension that is used for configuring tooltip behaviour,
  // configured so that the tooltip parent is the document body,
  // to avoid cutting off tooltips which overflow the editor.
  tooltips({
    parent: document.body,
    tooltipSpace(view) {
      const { top, bottom } = view.scrollDOM.getBoundingClientRect()

      return {
        top,
        left: 0,
        bottom,
        right: window.innerWidth,
      }
    },
  }),
  keymaps,
  goToLinePanel(),
  filterCharacters(),

  // NOTE: `autoComplete` needs to be before `keybindings` so that arrow key handling
  // in the autocomplete pop-up takes precedence over Vim/Emacs key bindings
  autoComplete({
    enabled: options.settings.autoComplete,
  }),

  // NOTE: `keybindings` needs to be before `language` so that Vim/Emacs bindings take
  // precedence over language-specific keyboard shortcuts
  keybindings(),

  docName(options.docName),

  // NOTE: `annotations` needs to be before `language`
  annotations(),
  language(options.docName, options.metadata, options.settings),
  indentUnit.of('    '), // 4 spaces
  theme(options.theme),
  realtime(options.onLocalChange),
  cursorPosition({ currentDoc: options.currentDoc }),
  scrollPosition({ currentDoc: options.currentDoc }, options.visual),
  cursorHighlights(),
  autoPair(options.settings),
  editable(),
  search(options.initialSearchQuery),
  phrases(options.phrases),
  shortcuts,
  symbolPalette(),
  mathPreview(options.settings.mathPreview),
  reviewTooltip(),
  spelling(options.spelling),
  // NOTE: `emptyLineFiller` needs to be before `trackedChanges`,
  // so the decorations are added in the correct order.
  emptyLineFiller(),
  trackedChanges(),
  visual(options.docName, options.visual),
  toolbarPanel(),
  breadcrumbPanel(),
  verticalOverflow(),
  highlightActiveLine(options.visual.visual),
  // The built-in extension that highlights the active line in the gutter.
  highlightActiveLineGutter(),
  inlineBackground(options.visual.visual),
  // Report exceptions from inside the editor
  EditorView.exceptionSink.of(options.handleException),
  effectListeners(),
  geometryChangeEvent(),
  fileTreeItemDrop(),
  tooltipsReposition(),
  selectionListener(options.setEditorSelection),
  tabsListener(options.settings.editorTabs),
]
