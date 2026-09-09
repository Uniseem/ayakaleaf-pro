'use client'

/**
 * The document's outline, from features/outline.
 *
 * A tree of the sections in the open file, nested by level, with the one
 * the cursor is in highlighted. A click goes to the section; a double click
 * also shows it in the PDF.
 */

import { memo, useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Tooltip } from '@/components/ol/tooltip'
import { useOutlineContext } from '@/features/ide/contexts/outline-context'

export type OutlineItemData = {
  level: number
  title: string
  line: number
  children?: OutlineItemData[]
}

/** Nests a flat list of headings by level. */
function nestOutline(flat: { level: number; title: string; line: number }[]): OutlineItemData[] {
  const root: OutlineItemData[] = []
  const stack: OutlineItemData[] = []
  for (const item of flat) {
    const node: OutlineItemData = { level: item.level, title: item.title, line: item.line }
    while (stack.length > 0 && stack[stack.length - 1]!.level >= node.level) {
      stack.pop()
    }
    const parent = stack[stack.length - 1]
    if (parent) {
      parent.children = parent.children ?? []
      parent.children.push(node)
    } else {
      root.push(node)
    }
    stack.push(node)
  }
  return root
}

function getChildrenLines(children?: OutlineItemData[]): number[] {
  return (children || []).reduce<number[]>((lines, child) => lines.concat(getChildrenLines(child.children), child.line), [])
}

export const useOutlineState = useOutlineContext

export const OutlineContainer = memo(function OutlineContainer() {
  const { flatOutline, highlightedLine, jumpToLine, canShowOutline, outlineExpanded, toggleOutlineExpanded } = useOutlineContext()

  const outline = useMemo(() => (flatOutline ? nestOutline(flatOutline.items) : []), [flatOutline])

  return (
    <div className="outline-container">
      <OutlinePane
        outline={outline}
        isTexFile={canShowOutline}
        jumpToLine={jumpToLine}
        highlightedLine={highlightedLine}
        isPartial={flatOutline?.partial}
        expanded={outlineExpanded}
        toggleExpanded={toggleOutlineExpanded}
      />
    </div>
  )
})

const OutlinePane = memo(function OutlinePane({
  isTexFile,
  outline,
  jumpToLine,
  highlightedLine,
  isPartial = false,
  expanded,
  toggleExpanded,
}: {
  isTexFile: boolean
  outline: OutlineItemData[]
  jumpToLine: (line: number, syncToPdf: boolean) => void
  highlightedLine?: number
  isPartial?: boolean
  expanded?: boolean
  toggleExpanded: () => void
}) {
  const isOpen = Boolean(isTexFile && expanded)

  return (
    <div className={['outline-pane', !isTexFile ? 'outline-pane-disabled' : ''].filter(Boolean).join(' ')}>
      <div className="outline-header">
        <OutlineToggleButton toggleExpanded={toggleExpanded} expanded={expanded} isOpen={isOpen} isPartial={isPartial} isTexFile={isTexFile} />
      </div>
      {isOpen ? (
        <div className="outline-body">
          <OutlineRoot outline={outline} jumpToLine={jumpToLine} highlightedLine={highlightedLine} />
        </div>
      ) : null}
    </div>
  )
})

const OutlineToggleButton = memo(function OutlineToggleButton({
  isTexFile,
  toggleExpanded,
  expanded,
  isOpen,
  isPartial,
}: {
  isTexFile: boolean
  toggleExpanded: () => void
  expanded?: boolean
  isOpen: boolean
  isPartial: boolean
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      className="outline-header-expand-collapse-btn"
      disabled={!isTexFile}
      onClick={toggleExpanded}
      aria-label={expanded ? t('hide_outline') : t('show_outline')}
    >
      <MaterialIcon type={isOpen ? 'keyboard_arrow_down' : 'keyboard_arrow_right'} className="outline-caret-icon" />
      <h4 className="outline-header-name">{t('file_outline')}</h4>
      {isPartial ? (
        <Tooltip id="partial-outline" description={t('partial_outline_warning')} overlayProps={{ placement: 'top' }}>
          <span role="status" style={{ display: 'flex' }}>
            <MaterialIcon type="warning" accessibilityLabel={t('partial_outline_warning')} />
          </span>
        </Tooltip>
      ) : null}
    </button>
  )
})

function OutlineRoot({
  outline,
  jumpToLine,
  highlightedLine,
}: {
  outline: OutlineItemData[]
  jumpToLine: (line: number, syncToPdf: boolean) => void
  highlightedLine?: number
}) {
  const { t } = useTranslation()
  return (
    <div>
      {outline.length ? (
        <OutlineList outline={outline} jumpToLine={jumpToLine} isRoot highlightedLine={highlightedLine} containsHighlightedLine />
      ) : (
        <div className="outline-body-no-elements">
          {t('we_cant_find_any_sections_or_subsections_in_this_file')}.{' '}
          <a
            href="https://docs.overleaf.com/navigating-in-the-editor/selecting-and-managing-files"
            className="outline-body-link"
            target="_blank"
            rel="noopener noreferrer"
          >
            {t('find_out_more_about_the_file_outline')}
          </a>
        </div>
      )}
    </div>
  )
}

const OutlineList = memo(function OutlineList({
  outline,
  jumpToLine,
  isRoot,
  highlightedLine,
  containsHighlightedLine,
}: {
  outline: OutlineItemData[]
  jumpToLine: (line: number, syncToPdf: boolean) => void
  isRoot?: boolean
  highlightedLine?: number | null
  containsHighlightedLine?: boolean
}) {
  return (
    <ul className={['outline-item-list', isRoot ? 'outline-item-list-root' : ''].filter(Boolean).join(' ')} role={isRoot ? 'tree' : 'group'}>
      {outline.map((outlineItem, index) => {
        const matchesHighlightedLine = containsHighlightedLine && highlightedLine === outlineItem.line
        const itemContainsHighlightedLine =
          highlightedLine !== undefined &&
          highlightedLine !== null &&
          containsHighlightedLine &&
          getChildrenLines(outlineItem.children).includes(highlightedLine)
        return (
          <OutlineItem
            key={`${outlineItem.level}-${index}`}
            outlineItem={outlineItem}
            jumpToLine={jumpToLine}
            highlightedLine={matchesHighlightedLine || itemContainsHighlightedLine ? highlightedLine : null}
            matchesHighlightedLine={matchesHighlightedLine}
            containsHighlightedLine={itemContainsHighlightedLine}
          />
        )
      })}
    </ul>
  )
})

const OutlineItem = memo(function OutlineItem({
  outlineItem,
  jumpToLine,
  highlightedLine,
  matchesHighlightedLine,
  containsHighlightedLine,
}: {
  outlineItem: OutlineItemData
  jumpToLine: (line: number, syncToPdf: boolean) => void
  highlightedLine?: number | null
  matchesHighlightedLine?: boolean
  containsHighlightedLine?: boolean
}) {
  const [expanded, setExpanded] = useState(true)
  const titleElementRef = useRef<HTMLButtonElement>(null)
  const isHighlightedRef = useRef(false)

  const hasHighlightedChild = !expanded && containsHighlightedLine
  const isHighlighted = matchesHighlightedLine || hasHighlightedChild

  useEffect(() => {
    const wasHighlighted = isHighlightedRef.current
    isHighlightedRef.current = Boolean(isHighlighted)
    if (!wasHighlighted && isHighlighted && titleElementRef.current) {
      titleElementRef.current.scrollIntoView({ block: 'center' })
    }
  }, [isHighlighted])

  return (
    <li
      className={['outline-item', !outlineItem.children ? 'outline-item-no-children' : ''].filter(Boolean).join(' ')}
      aria-expanded={outlineItem.children ? expanded : undefined}
      role="treeitem"
      aria-current={isHighlighted}
      aria-label={outlineItem.title}
      translate="no"
    >
      <div className="outline-item-row">
        {outlineItem.children ? <OutlineItemToggleButton expanded={expanded} setExpanded={setExpanded} /> : null}
        <button
          type="button"
          className={['outline-item-link', isHighlighted ? 'outline-item-link-highlight' : ''].filter(Boolean).join(' ')}
          onClick={event => jumpToLine(outlineItem.line, event.detail === 2)}
          ref={titleElementRef}
        >
          {outlineItem.title}
        </button>
      </div>
      {expanded && outlineItem.children ? (
        <OutlineList
          outline={outlineItem.children}
          jumpToLine={jumpToLine}
          isRoot={false}
          highlightedLine={containsHighlightedLine ? highlightedLine : null}
          containsHighlightedLine={containsHighlightedLine}
        />
      ) : null}
    </li>
  )
})

const OutlineItemToggleButton = memo(function OutlineItemToggleButton({
  expanded,
  setExpanded,
}: {
  expanded: boolean
  setExpanded: Dispatch<SetStateAction<boolean>>
}) {
  const { t } = useTranslation()
  return (
    <button type="button" className="outline-item-expand-collapse-btn" onClick={() => setExpanded(value => !value)} aria-label={expanded ? t('collapse') : t('expand')}>
      <MaterialIcon type={expanded ? 'keyboard_arrow_down' : 'keyboard_arrow_right'} className="outline-caret-icon" />
    </button>
  )
})
