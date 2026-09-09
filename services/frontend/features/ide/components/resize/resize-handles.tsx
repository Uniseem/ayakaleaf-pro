'use client'

/**
 * The bars between panels, from ide-react/components/resize.
 *
 * A 7px strip with a grip drawn on it, and a toggler in the middle that
 * collapses the panel on one side of it. Dragging is reported only once the
 * pointer moves, so a click on the toggler is not a resize.
 */

import { useEffect, useState, type ReactNode } from 'react'
import { PanelResizeHandle, type PanelResizeHandleProps } from 'react-resizable-panels'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'

export function HorizontalResizeHandle({
  children,
  resizable = true,
  onDoubleClick,
  onDragging,
  className,
  ...props
}: {
  children?: ReactNode
  resizable?: boolean
  onDoubleClick?: () => void
} & PanelResizeHandleProps) {
  const { t } = useTranslation()
  const [isDragging, setIsDragging] = useState(false)

  function handleDragging(dragging: boolean) {
    if (isDragging || resizable) {
      setIsDragging(dragging)
    }
  }

  useEffect(() => {
    if (isDragging) {
      const handlePointerMove = () => onDragging?.(true)
      document.addEventListener('pointermove', handlePointerMove)
      return () => document.removeEventListener('pointermove', handlePointerMove)
    }
    onDragging?.(false)
  }, [isDragging, onDragging])

  return (
    <PanelResizeHandle disabled={!resizable && !isDragging} className={className} {...props} onDragging={handleDragging}>
      <div
        className={['horizontal-resize-handle', resizable ? 'horizontal-resize-handle-enabled' : ''].filter(Boolean).join(' ')}
        title={t('resize')}
        onDoubleClick={() => onDoubleClick?.()}
      >
        {children}
      </div>
    </PanelResizeHandle>
  )
}

export function HorizontalToggler({
  id,
  isOpen,
  setIsOpen,
  togglerType,
  tooltipWhenOpen,
  tooltipWhenClosed,
}: {
  id: string
  isOpen: boolean
  setIsOpen: (isOpen: boolean) => void
  togglerType: 'west' | 'east'
  tooltipWhenOpen: string
  tooltipWhenClosed: string
}) {
  const description = isOpen ? tooltipWhenOpen : tooltipWhenClosed

  return (
    <Tooltip id={id} description={description} overlayProps={{ placement: togglerType === 'east' ? 'left' : 'right' }}>
      <button
        type="button"
        className={['custom-toggler', `custom-toggler-${togglerType}`, isOpen ? 'custom-toggler-open' : 'custom-toggler-closed'].join(' ')}
        aria-label={description}
        title=""
        onClick={() => setIsOpen(!isOpen)}
      >
        <MaterialIcon
          type={(togglerType === 'west' && isOpen) || (togglerType === 'east' && !isOpen) ? 'chevron_left' : 'chevron_right'}
        />
      </button>
    </Tooltip>
  )
}

export function VerticalResizeHandle(props: PanelResizeHandleProps) {
  const { t } = useTranslation()
  return (
    <PanelResizeHandle {...props}>
      <div
        className={['vertical-resize-handle', !props.disabled ? 'vertical-resize-handle-enabled' : ''].filter(Boolean).join(' ')}
        title={t('resize')}
      />
    </PanelResizeHandle>
  )
}
