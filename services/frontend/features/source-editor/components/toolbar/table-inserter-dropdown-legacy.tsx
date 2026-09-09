'use client'

import { memo, useCallback, useRef, useState } from 'react'
import * as commands from '../../extensions/toolbar/commands'
import { useTranslation } from '@/lib/i18n'
import useDropdown from '../../hooks/use-dropdown'
import { useCodeMirrorViewContext } from '../codemirror-context'
import { Tooltip } from '@/components/ol/tooltip'
import { Overlay, Popover } from '@/components/ol/overlay'
import MaterialIcon from '@/components/ol/material-icon'
import cx from '@/lib/cx'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'

export const LegacyTableDropdown = memo(function LegacyTableDropdown() {
  const { t } = useTranslation()
  const { open, onToggle, ref } = useDropdown()
  const view = useCodeMirrorViewContext()
  const target = useRef<HTMLButtonElement>(null)

  const onSizeSelected = useCallback(
    (sizeX: number, sizeY: number) => {
      onToggle(false)
      commands.insertTable(view, sizeX, sizeY)
      emitToolbarEvent(view, 'table-generator-insert-table')
      view.focus()
    },
    [view, onToggle]
  )

  return (
    <>
      <Tooltip hidden={open} id="toolbar-table" description={<div>{t('toolbar_insert_table')}</div>} overlayProps={{ placement: 'bottom' }}>
        <button
          type="button"
          className="ol-cm-toolbar-button"
          aria-label={t('toolbar_insert_table')}
          onMouseDown={event => {
            event.preventDefault()
            event.stopPropagation()
          }}
          onClick={() => {
            onToggle(!open)
          }}
          ref={target}
        >
          <MaterialIcon type="table_chart" />
        </button>
      </Tooltip>
      <Overlay
        show={open}
        target={target.current}
        placement="bottom"
        container={view.dom}
        containerPadding={0}
        transition
        rootClose
        onHide={() => onToggle(false)}
      >
        <Popover id="toolbar-table-menu" ref={ref} className="ol-cm-toolbar-button-menu-popover ol-cm-toolbar-button-menu-popover-unstyled">
          <div className="ol-cm-toolbar-table-grid-popover">
            <SizeGrid sizeX={10} sizeY={10} onSizeSelected={onSizeSelected} />
          </div>
        </Popover>
      </Overlay>
    </>
  )
})

const range = (start: number, end: number) => Array.from({ length: end - start + 1 }, (v, k) => k + start)

export const SizeGrid = ({
  sizeX,
  sizeY,
  onSizeSelected,
}: {
  sizeX: number
  sizeY: number
  onSizeSelected: (sizeX: number, sizeY: number) => void
}) => {
  const [currentSize, setCurrentSize] = useState<{
    sizeX: number
    sizeY: number
  }>({ sizeX: 0, sizeY: 0 })
  const { t } = useTranslation()
  let label = t('toolbar_table_insert_table_lowercase')
  if (currentSize.sizeX > 0 && currentSize.sizeY > 0) {
    label = t('toolbar_table_insert_size_table', {
      size: `${currentSize.sizeY}×${currentSize.sizeX}`,
    })
  }
  return (
    <>
      <div className="ol-cm-toolbar-table-size-label">{label}</div>
      <table
        className="ol-cm-toolbar-table-grid"
        onMouseLeave={() => {
          setCurrentSize({ sizeX: 0, sizeY: 0 })
        }}
      >
        <tbody>
          {range(1, sizeY).map(y => (
            <tr key={y}>
              {range(1, sizeX).map(x => (
                <td
                  className={cx('ol-cm-toolbar-table-cell', {
                    active: currentSize.sizeX >= x && currentSize.sizeY >= y,
                  })}
                  key={x}
                  onMouseEnter={() => {
                    setCurrentSize({ sizeX: x, sizeY: y })
                  }}
                  onMouseUp={() => onSizeSelected(x, y)}
                />
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
