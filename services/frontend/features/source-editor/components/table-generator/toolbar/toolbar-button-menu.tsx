import { FC, memo, useRef } from 'react'
import useDropdown from '@/features/source-editor/hooks/use-dropdown'
import { ListGroup as OLListGroup } from '@/components/ol/list-group'
import { Overlay as OLOverlay } from '@/components/ol/overlay'
import { Popover as OLPopover } from '@/components/ol/overlay'
import { Tooltip as OLTooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { useTabularContext } from '../contexts/tabular-context'

export const ToolbarButtonMenu: FC<
  React.PropsWithChildren<{
    id: string
    label: string
    icon: string
    disabled?: boolean
    disabledLabel?: string
  }>
> = memo(function ButtonMenu({
  icon,
  id,
  label,
  children,
  disabled,
  disabledLabel,
}) {
  const target = useRef<any>(null)
  const { open, onToggle, ref } = useDropdown()
  const { ref: tableContainerRef } = useTabularContext()

  const button = (
    <button
      type="button"
      className="table-generator-toolbar-button table-generator-toolbar-button-menu"
      aria-label={label}
      onMouseDown={event => {
        event.preventDefault()
        event.stopPropagation()
      }}
      onClick={() => {
        onToggle(!open)
      }}
      disabled={disabled}
      aria-disabled={disabled}
      ref={target}
    >
      <MaterialIcon type={icon} />
      <MaterialIcon type="expand_more" />
    </button>
  )

  const overlay = tableContainerRef.current && (
    <OLOverlay
      show={open}
      target={target.current}
      placement="bottom"
      container={tableContainerRef.current}
      containerPadding={0}
      transition
      rootClose
      onHide={() => onToggle(false)}
    >
      <OLPopover
        id={`${id}-menu`}
        ref={ref}
        className="table-generator-button-menu-popover"
      >
        <OLListGroup
          role="menu"
          onClick={() => {
            onToggle(false)
          }}
          className="d-block"
        >
          {children}
        </OLListGroup>
      </OLPopover>
    </OLOverlay>
  )

  return (
    <>
      <OLTooltip
        hidden={open}
        id={id}
        description={
          <div>{disabled && disabledLabel ? disabledLabel : label}</div>
        }
        overlayProps={{ placement: 'bottom' }}
      >
        {button}
      </OLTooltip>
      {overlay}
    </>
  )
})
