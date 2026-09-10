import { memo, useCallback } from 'react'
import { useTranslation } from '@/lib/i18n'
import OLTooltip from '@/components/ol/tooltip'
import { DropdownItem } from '@/components/ol/dropdown'
import MaterialIcon from '@/components/ol/material-icon'
import { useLayout } from '@/features/ide/contexts/layout-context'

/**
 * The language the suggestions came from, at the foot of the menu.
 *
 * It is a button, not a label: somebody looking at a wrong suggestion is
 * usually looking at the wrong language, and this is the shortest way from
 * noticing that to changing it.
 */
export const SpellingSuggestionsLanguage = memo<{
  language: { name: string }
  handleClose: (focus: boolean) => void
}>(({ language, handleClose }) => {
  const { t } = useTranslation()
  const { setLeftMenuShown } = useLayout()

  const handleClick = useCallback(() => {
    setLeftMenuShown(true)
    handleClose(false)
  }, [handleClose, setLeftMenuShown])

  return (
    <OLTooltip
      id="spell-check-client-tooltip"
      description={t('change_language')}
      overlayProps={{ placement: 'right', delay: 100 }}
    >
      <span>
        <DropdownItem
          className="d-flex gap-2 align-items-center"
          onClick={handleClick}
        >
          <MaterialIcon type="settings" />
          <span>{language.name}</span>
        </DropdownItem>
      </span>
    </OLTooltip>
  )
})
SpellingSuggestionsLanguage.displayName = 'SpellingSuggestionsLanguage'
