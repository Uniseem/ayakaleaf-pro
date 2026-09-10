import {
  FC,
  HTMLProps,
  PropsWithChildren,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { useTranslation } from '@/lib/i18n'
import { EditorView } from '@codemirror/view'
import { PastedContent } from '../../extensions/visual/pasted-content'
import { useEventListener } from '@/lib/hooks'

import MaterialIcon from '@/components/ol/material-icon'
import { Overlay, Popover } from '@/components/ol/overlay'
import { isMac } from '@/lib/os'

/**
 * The menu offered next to content pasted from a web page.
 *
 * Pasting HTML into the visual editor converts it to LaTeX, and that is not
 * always what the writer wanted -- so the conversion is offered as something
 * to undo, next to the paste, until they move on. Pasting again while the menu
 * is open repeats the choice, which is why it listens for paste itself.
 */
export const PastedContentMenu: FC<{
  insertPastedContent: (
    view: EditorView,
    pastedContent: PastedContent,
    formatted: boolean
  ) => void
  pastedContent: PastedContent
  view: EditorView
  formatted: boolean
}> = ({ view, insertPastedContent, pastedContent, formatted }) => {
  const [menuOpen, setMenuOpen] = useState(false)
  const toggleButtonRef = useRef<HTMLButtonElement | null>(null)
  const { t } = useTranslation()

  // record whether the Shift key is currently down, for use in the `paste` event handler
  const shiftRef = useRef(false)
  useEventListener(
    'keydown',
    useCallback((event: KeyboardEvent) => {
      shiftRef.current = event.shiftKey
    }, [])
  )

  useEffect(() => {
    if (menuOpen) {
      const abortController = new AbortController()
      view.dom.addEventListener(
        'paste',
        event => {
          event.preventDefault()
          event.stopPropagation()
          insertPastedContent(view, pastedContent, !shiftRef.current)
          setMenuOpen(false)
        },
        { signal: abortController.signal, capture: true }
      )
      return () => {
        abortController.abort()
      }
    }
  }, [view, menuOpen, pastedContent, insertPastedContent])

  // TODO: keyboard navigation

  return (
    <>
      <button
        ref={toggleButtonRef}
        type="button"
        id="pasted-content-menu-button"
        aria-haspopup="true"
        aria-expanded={menuOpen}
        aria-controls="pasted-content-menu"
        aria-label={t('paste_options')}
        className="ol-cm-pasted-content-menu-toggle"
        tabIndex={0}
        onMouseDown={event => event.preventDefault()}
        onClick={() => setMenuOpen(isOpen => !isOpen)}
        style={{ userSelect: 'none' }}
      >
        <MaterialIcon type="content_copy" />
        <MaterialIcon type="expand_more" />
      </button>

      {menuOpen && (
        <Overlay
          show
          onHide={() => setMenuOpen(false)}
          transition={false}
          container={view.scrollDOM}
          containerPadding={0}
          placement="bottom"
          rootClose
          target={toggleButtonRef?.current}
        >
          <Popover
            id="popover-pasted-content-menu"
            className="ol-cm-pasted-content-menu-popover"
          >
            <div
              className="ol-cm-pasted-content-menu"
              id="pasted-content-menu"
              role="menu"
              aria-labelledby="pasted-content-menu-button"
            >
              <MenuItem
                onClick={() => {
                  insertPastedContent(view, pastedContent, true)
                  setMenuOpen(false)
                }}
              >
                <span style={{ visibility: formatted ? 'visible' : 'hidden' }}>
                  <MaterialIcon type="check" />
                </span>
                <span className="ol-cm-pasted-content-menu-item-label">
                  {t('paste_with_formatting')}
                </span>
                <span className="ol-cm-pasted-content-menu-item-shortcut">
                  {isMac ? '⌘V' : 'Ctrl+V'}
                </span>
              </MenuItem>

              <MenuItem
                onClick={() => {
                  insertPastedContent(view, pastedContent, false)
                  setMenuOpen(false)
                }}
              >
                <span style={{ visibility: formatted ? 'hidden' : 'visible' }}>
                  <MaterialIcon type="check" />
                </span>
                <span className="ol-cm-pasted-content-menu-item-label">
                  {t('paste_without_formatting')}
                </span>
                <span className="ol-cm-pasted-content-menu-item-shortcut">
                  {isMac ? '⇧⌘V' : 'Ctrl+Shift+V'}
                </span>
              </MenuItem>
            </div>
          </Popover>
        </Overlay>
      )}
    </>
  )
}

const MenuItem = ({
  children,
  ...buttonProps
}: PropsWithChildren<HTMLProps<HTMLButtonElement>>) => (
  <button
    {...buttonProps}
    type="button"
    role="menuitem"
    className="ol-cm-pasted-content-menu-item"
  >
    {children}
  </button>
)
