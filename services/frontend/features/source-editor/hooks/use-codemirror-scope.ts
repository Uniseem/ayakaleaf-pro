import { useCallback, useEffect, useRef } from 'react'
import { EditorState, type EditorSelection } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import type { SearchQuery } from '@codemirror/search'
import { createExtensions } from '../extensions'
import { setEditorTheme, setOptionsTheme } from '../extensions/theme'
import { restoreCursorPosition, setCursorLineAndScroll, setCursorPositionAndScroll } from '../extensions/cursor-position'
import { setAnnotations, showCompileLogDiagnostics, type Annotation } from '../extensions/annotations'
import { setCursorHighlights, type Highlight } from '../extensions/cursor-highlights'
import { setLanguage, setMetadata, setSyntaxValidation, type Metadata } from '../extensions/language'
import { restoreScrollPosition } from '../extensions/scroll-position'
import { setEditable } from '../extensions/editable'
import { setAutoPair } from '../extensions/auto-pair'
import { setAutoComplete } from '../extensions/auto-complete'
import { usePhrases } from './use-phrases'
import { setPhrases } from '../extensions/phrases'
import { setMathPreview } from '../extensions/math-preview'
import { setSpellCheckLanguage } from '../extensions/spelling'
import { useHunspell } from './use-hunspell'
import { setKeybindings } from '../extensions/keybindings'
import { setVisual } from '../extensions/visual/visual'
import { setDocName } from '../extensions/doc-name'
import { setNonBlinkingCursor } from '../extensions/non-blinking-cursor'
import { setEditorTabs } from '../extensions/tabs-listener'
import { beforeChangeDocEffect } from '../extensions/before-change-doc'
import { applyRemoteText } from '../extensions/realtime'
import { rejectionChanges, setTrackedChanges } from '../tracked-changes'
import { isVisualEditorAvailable } from '../utils/misc'
import { debugConsole } from '@/lib/debug'
import { useEventListener } from '@/lib/hooks'
import { getHueForUserId } from '@/lib/colors'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useSettings, type UserSettings } from '@/features/ide/contexts/settings-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { previewByPath } from '@/features/file-tree/util/preview-path'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { useMetadataContext } from '@/features/ide/contexts/metadata-context'
import { useEditorPropertiesContext } from '@/features/ide/contexts/editor-properties-context'
import { useReview } from '@/features/ide/contexts/review-context'

type GotoLineDetail = {
  /** 0-based */
  line: number
  column?: number
  selectText?: string
  syncToPdf?: boolean
  docId?: string
}


/** The other people's cursors in this document. */
const highlightsForDoc = (
  others: { id: string; name: string; docId?: string; row?: number; column?: number }[],
  docId: string
): Highlight[] => {
  const highlights: Highlight[] = []
  for (const user of others) {
    if (user.docId === docId && typeof user.row === 'number') {
      highlights.push({
        cursor: { row: user.row, column: user.column ?? 0 },
        hue: getHueForUserId(user.id),
        label: user.name,
      })
    }
  }
  return highlights
}

const fontFamilyFor = (value: UserSettings['fontFamily']) => value

/**
 * Keeps the CodeMirror view in step with everything around it: the open
 * document, the settings, the compile log, the other people's cursors.
 *
 * Each thing that can change is written into a ref and pushed into the
 * editor when it does, so the editor state is rebuilt only when the document
 * changes -- the one time that is unavoidable.
 */
function useCodeMirrorScope(view: EditorView) {
  const editor = useEditor()
  const { current, revision, change, editable, rememberPosition } = editor
  const { files, canWrite, canReview, projectId, entryByPath, project } = useProject()
  const { logEntryAnnotations, compiling, stale, markEdited } = useCompile()
  const { others, reportPosition } = useConnection()
  const metadata = useMetadataContext()
  const settings = useSettings()
  const {
    fontFamily,
    fontSize,
    lineHeight,
    autoComplete,
    autoPairDelimiters,
    keybindings: mode,
    syntaxValidation,
    mathPreview,
    editorTabs,
    nonBlinkingCursor,
    overallTheme: activeOverallTheme,
    editorTheme,
  } = settings
  const { showVisual: visual, trackChanges } = useEditorPropertiesContext()
  const review = useReview()

  const openDocName = current?.name
  const docId = current?.id

  // build the translation phrases
  const phrases = usePhrases()
  const phrasesRef = useRef(phrases)

  // initialise the local state

  const themeRef = useRef({
    fontFamily: fontFamilyFor(fontFamily),
    fontSize,
    lineHeight,
    activeOverallTheme,
    editorTheme,
  })

  useEffect(() => {
    themeRef.current = {
      fontFamily: fontFamilyFor(fontFamily),
      fontSize,
      lineHeight,
      activeOverallTheme,
      editorTheme,
    }

    view.dispatch(
      setOptionsTheme({
        fontFamily: fontFamilyFor(fontFamily),
        fontSize,
        lineHeight,
        activeOverallTheme,
      })
    )

    setEditorTheme(editorTheme).then(spec => {
      view.dispatch(spec)
    })
  }, [view, fontFamily, fontSize, lineHeight, activeOverallTheme, editorTheme])

  const settingsRef = useRef({
    autoComplete,
    autoPairDelimiters,
    mode,
    syntaxValidation,
    mathPreview,
    editorTabs,
    nonBlinkingCursor,
  })

  const docNameRef = useRef(openDocName ?? '')

  // tracked changes come from the review context
  useEffect(() => {
    window.setTimeout(() => {
      view.dispatch({ effects: setTrackedChanges.of(review.ranges.changes) })
    })
  }, [view, review.ranges, revision])

  // typing goes to the document session, and marks the PDF as behind the text
  const changeRef = useRef(change)
  const markEditedRef = useRef(markEdited)
  useEffect(() => {
    changeRef.current = change
    markEditedRef.current = markEdited
  }, [change, markEdited])

  // the project metadata, mostly for use in autocomplete
  const metadataRef = useRef<Metadata>({
    ...metadata,
    fileTreeData: files,
  })

  // listen to project metadata (commands, labels, package names, reference keys) updates
  useEffect(() => {
    metadataRef.current = { ...metadataRef.current, ...metadata }
    window.setTimeout(() => {
      view.dispatch(setMetadata(metadataRef.current))
    })
  }, [view, metadata])

  // listen to project file tree updates
  useEffect(() => {
    metadataRef.current.fileTreeData = files
    window.setTimeout(() => {
      view.dispatch(setMetadata(metadataRef.current))
    })
  }, [view, files])

  const editableRef = useRef(editable && (canWrite || canReview))

  // Spell check is off for somebody who cannot write: there is nothing they
  // could do about a misspelling, and underlining it is only noise.
  // The project's language if it has one, otherwise this person's own: a
  // project created before they chose a language should still be checked in
  // the language they read.
  const spellCheckLanguage =
    canWrite || canReview
      ? project.spellCheckLanguage || settings.spellCheckLanguage || ''
      : ''
  const hunspellManager = useHunspell(spellCheckLanguage)

  const spellingRef = useRef({ spellCheckLanguage, hunspellManager })

  useEffect(() => {
    spellingRef.current = { spellCheckLanguage, hunspellManager }
    window.setTimeout(() => {
      view.dispatch(setSpellCheckLanguage(spellingRef.current))
    })
  }, [view, spellCheckLanguage, hunspellManager])

  const showVisual = visual && !!openDocName && isVisualEditorAvailable(openDocName)

  // Resolving a graphics path needs the file tree, which changes as files are
  // added; the widgets hold on to this function, so it reads through a ref
  // rather than being rebuilt and invalidating every figure.
  const entryByPathRef = useRef(entryByPath)
  entryByPathRef.current = entryByPath

  const visualRef = useRef({
    visual: showVisual,
    previewByPath: (path: string) =>
      previewByPath(entryByPathRef.current, projectId, path),
  })

  // Persist the search query when the document changes by keeping a
  // reference to the search query in sync with the editor state
  const searchQueryRef = useRef<SearchQuery | null>(null)
  useEventListener(
    'search-panel-before-doc-change' as keyof WindowEventMap,
    useCallback((event: Event) => {
      searchQueryRef.current = (event as CustomEvent<SearchQuery | null>).detail
    }, [])
  )

  const handleException = useCallback((exception: unknown) => {
    debugConsole.error('editor exception', exception)
  }, [])

  const setEditorSelection = useCallback((selection: EditorSelection | undefined) => {
    window.dispatchEvent(new CustomEvent('editor:selection', { detail: selection }))
  }, [])

  // The text the editor was last given, to tell a remote edit from a render.
  const heldRevision = useRef(-1)
  const heldDocId = useRef<string | null>(null)

  // create a new state when the document changes

  useEffect(() => {
    if (current && (heldDocId.current !== current.id || heldRevision.current !== revision)) {
      debugConsole.log('creating new editor state')

      // Warn any interested extension that the document is about to change,
      // allowing it to perform any necessary actions before creating the new
      // state. destroy() is too late because the new state is already created
      view.dispatch({
        effects: beforeChangeDocEffect.of(null),
      })

      heldDocId.current = current.id
      heldRevision.current = revision
      docNameRef.current = current.name

      const state = EditorState.create({
        doc: current.content,
        extensions: createExtensions({
          currentDoc: { doc_id: current.id },
          docName: current.name,
          theme: themeRef.current,
          metadata: metadataRef.current,
          settings: settingsRef.current,
          phrases: phrasesRef.current,
          visual: visualRef.current,
      spelling: spellingRef.current,
          initialSearchQuery: searchQueryRef.current,
          handleException,
          onLocalChange: text => {
            changeRef.current(text)
            markEditedRef.current()
          },
          setEditorSelection,
        }),
      })
      view.setState(state)

      // synchronous config
      view.dispatch(
        restoreCursorPosition(state.doc, current.id),
        setEditable(editableRef.current),
        setOptionsTheme(themeRef.current)
      )

      // asynchronous config
      setEditorTheme(themeRef.current.editorTheme).then(spec => {
        view.dispatch(spec)
      })

      setKeybindings(settingsRef.current.mode).then(spec => {
        view.dispatch(spec)
      })

      if (!visualRef.current.visual) {
        window.setTimeout(() => {
          view.dispatch(restoreScrollPosition())
          view.focus()
        })
      }
    }
    // IMPORTANT: This effect must not depend on anything variable apart from
    // the document, as the editor state is recreated when the effect runs.
  }, [view, current?.id, revision, handleException, setEditorSelection]) // eslint-disable-line react-hooks/exhaustive-deps

  // the same document, changed by somebody else
  const content = current?.content
  useEffect(() => {
    if (content !== undefined && heldDocId.current === docId) {
      applyRemoteText(view, content)
    }
  }, [view, content, docId])

  useEffect(() => {
    if (openDocName) {
      docNameRef.current = openDocName

      window.setTimeout(() => {
        view.dispatch(setDocName(openDocName), setLanguage(openDocName, metadataRef.current, settingsRef.current.syntaxValidation))
      })
    }
  }, [view, openDocName])

  useEffect(() => {
    visualRef.current.visual = showVisual
    window.setTimeout(() => {
      view.dispatch(setVisual(visualRef.current))
      view.dispatch({
        effects: EditorView.scrollIntoView(view.state.selection.main.head),
      })
      window.dispatchEvent(new Event('editor:visual-switch'))
    })
  }, [view, showVisual])

  useEffect(() => {
    editableRef.current = editable && (canWrite || canReview)
    window.setTimeout(() => {
      view.dispatch(setEditable(editableRef.current)) // the editor needs to be locked when there's a problem saving data
    })
  }, [view, editable, canWrite, canReview])

  useEffect(() => {
    phrasesRef.current = phrases
    window.setTimeout(() => {
      view.dispatch(setPhrases(phrases))
    })
  }, [view, phrases])

  // listen to editor settings updates
  useEffect(() => {
    settingsRef.current.autoPairDelimiters = autoPairDelimiters
    window.setTimeout(() => {
      view.dispatch(setAutoPair(autoPairDelimiters))
    })
  }, [view, autoPairDelimiters])

  useEffect(() => {
    settingsRef.current.autoComplete = autoComplete
    window.setTimeout(() => {
      view.dispatch(setAutoComplete({ enabled: autoComplete }))
    })
  }, [view, autoComplete])

  useEffect(() => {
    settingsRef.current.mode = mode
    setKeybindings(mode).then(spec => {
      window.setTimeout(() => {
        view.dispatch(spec)
      })
    })
  }, [view, mode])

  useEffect(() => {
    settingsRef.current.syntaxValidation = syntaxValidation
    window.setTimeout(() => {
      view.dispatch(setSyntaxValidation(syntaxValidation))
    })
  }, [view, syntaxValidation])

  useEffect(() => {
    settingsRef.current.mathPreview = mathPreview
    window.setTimeout(() => {
      view.dispatch(setMathPreview(mathPreview))
    })
  }, [view, mathPreview])

  useEffect(() => {
    settingsRef.current.editorTabs = editorTabs
    window.setTimeout(() => {
      view.dispatch(setEditorTabs(editorTabs))
    })
  }, [view, editorTabs])

  useEffect(() => {
    settingsRef.current.nonBlinkingCursor = nonBlinkingCursor
    window.setTimeout(() => {
      view.dispatch(setNonBlinkingCursor(nonBlinkingCursor))
    })
  }, [view, nonBlinkingCursor])

  // the tracked-changes mode is decided by the review context
  useEffect(() => {
    debugConsole.log('track changes', trackChanges)
  }, [trackChanges])

  // select and scroll to a position on the ide:goto-line event (from synctex, the outline, search)
  const currentRef = useRef(current)
  currentRef.current = current
  useEventListener(
    'ide:goto-line' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const options = (event as CustomEvent<GotoLineDetail>).detail
        if (!options) {
          return
        }
        if (options.docId && currentRef.current && options.docId !== currentRef.current.id) {
          // the document is still being opened; the sender will ask again
          return
        }
        setCursorLineAndScroll(view, options.line + 1, options.column, options.selectText)
        if (options.syncToPdf && currentRef.current) {
          const line = options.line + 1
          window.dispatchEvent(
            new CustomEvent('ide:show-in-pdf', {
              detail: { file: currentRef.current.path, line },
            })
          )
        }
      },
      [view]
    )
  )

  // select and scroll to a position on the ide:goto-offset event (from the review panel)
  useEventListener(
    'ide:goto-offset' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const options = (event as CustomEvent<{ gotoOffset: number }>).detail
        setCursorPositionAndScroll(view, options.gotoOffset)
      },
      [view]
    )
  )

  // where the cursor is, for "show in PDF" and for the others
  const lastReport = useRef(0)
  useEventListener(
    'cursor:editor:update' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const { row, column } = (event as CustomEvent<{ row: number; column: number }>).detail
        const open = currentRef.current
        if (!open) {
          return
        }
        rememberPosition(view.state.selection.main.head, view.scrollDOM.scrollTop)
        const now = Date.now()
        if (now - lastReport.current >= 300) {
          lastReport.current = now
          reportPosition(open.id, row, column)
        }
      },
      [view, rememberPosition, reportPosition]
    )
  )

  // the PDF pane asks where the cursor is
  useEventListener(
    'ide:sync-to-pdf' as keyof WindowEventMap,
    useCallback(() => {
      const open = currentRef.current
      if (!open) {
        return
      }
      const line = view.state.doc.lineAt(view.state.selection.main.head)
      window.dispatchEvent(
        new CustomEvent('ide:show-in-pdf', {
          detail: { file: open.path, line: line.number },
        })
      )
    }, [view])
  )

  // rejecting a tracked change is an ordinary edit made here
  useEventListener(
    'ide:reject-changes' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const detail = (event as CustomEvent<{ changes: Parameters<typeof rejectionChanges>[0] }>).detail
        if (!detail) {
          return
        }
        const changes = rejectionChanges(detail.changes, view.state.doc.length)
        if (changes.length > 0) {
          view.dispatch({ changes })
        }
      },
      [view]
    )
  )

  // enable the compile log linter a) when "Code Check" is off, b) when the project hasn't changed and isn't compiling.
  const enableCompileLogLinter = !syntaxValidation || (!stale && !compiling)

  // store enableCompileLogLinter in a ref for use in useEffect
  const enableCompileLogLinterRef = useRef(enableCompileLogLinter)

  useEffect(() => {
    enableCompileLogLinterRef.current = enableCompileLogLinter
  }, [enableCompileLogLinter])

  // enable/disable the compile log linter as appropriate
  useEffect(() => {
    window.setTimeout(() => {
      view.dispatch(showCompileLogDiagnostics(enableCompileLogLinter))
    })
  }, [view, enableCompileLogLinter])

  // set the compile log annotations when they change
  useEffect(() => {
    if (docId) {
      const annotations = logEntryAnnotations?.[docId] ?? []

      window.setTimeout(() => {
        view.dispatch(
          setAnnotations(view.state, annotations),
          // reconfigure the compile log lint source, so it runs once with the new data
          showCompileLogDiagnostics(enableCompileLogLinterRef.current)
        )
      })
    }
  }, [view, docId, logEntryAnnotations])

  useEffect(() => {
    if (docId) {
      const items = highlightsForDoc(others, docId)
      window.setTimeout(() => {
        view.dispatch(setCursorHighlights(items))
      })
    }
  }, [view, others, docId])

  useEventListener(
    'editor:focus' as keyof WindowEventMap,
    useCallback(() => {
      view.focus()
    }, [view])
  )
}

export default useCodeMirrorScope
