'use client'

import { memo, useCallback } from 'react'
import { useTranslation } from '@/lib/i18n'
import cx from '@/lib/cx'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useStopOnFirstError } from '../hooks/use-stop-on-first-error'
import Tooltip from '@/components/ol/tooltip'
import {
  DropdownToggleCustom,
  Dropdown,
  DropdownDivider,
  DropdownHeader,
  DropdownItem,
  DropdownMenu,
  DropdownToggle,
} from '@/components/ol/dropdown'
import { Button } from '@/components/ol/button'
import { ButtonGroup } from '@/components/ol/list-group'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { isMac } from '@/lib/hooks'

/**
 * The Recompile button and the menu of compile settings beside it, from
 * pdf-preview/components/pdf-compile-button.
 */
function PdfCompileButton() {
  const {
    animateCompileDropdownArrow,
    autoCompile,
    compiling,
    draft,
    hasChanges,
    setAutoCompile,
    setDraft,
    setStopOnValidationError,
    stopOnFirstError,
    stopOnValidationError,
    startCompile,
    stopCompile,
    recompileFromScratch,
  } = useCompile()
  const { enableStopOnFirstError, disableStopOnFirstError } = useStopOnFirstError()

  const { t } = useTranslation()

  const { detachRole } = useLayout()

  const modifierKey = isMac() ? 'Cmd' : 'Ctrl'

  const fromScratch = useCallback(() => {
    recompileFromScratch()
  }, [recompileFromScratch])

  const tooltipElement = (
    <>
      {t('recompile_pdf')} <span className="keyboard-shortcut">({modifierKey} + Enter)</span>
    </>
  )

  const dropdownToggleClassName = cx(
    {
      'detach-compile-button-animate': animateCompileDropdownArrow,
      'btn-striped-animated': hasChanges,
    },
    'no-left-border',
    'dropdown-button-toggle',
    'compile-dropdown-toggle'
  )

  const buttonClassName = cx('align-items-center py-0 no-left-radius px-3', 'compile-button', {
    'btn-striped-animated': hasChanges,
  })

  useCommandProvider(
    () => [
      {
        id: 'compile',
        handler: () => startCompile(),
        label: t('recompile'),
        disabled: compiling,
      },
      {
        id: 'stop-compile',
        handler: () => stopCompile(),
        label: t('stop_compile'),
        disabled: !compiling,
      },
      {
        id: 'recompile-from-scratch',
        handler: fromScratch,
        label: t('recompile_from_scratch'),
        disabled: compiling,
      },
    ],
    [startCompile, t, compiling, stopCompile, fromScratch]
  )

  return (
    <Dropdown as={ButtonGroup} className="compile-button-group">
      <Tooltip
        description={tooltipElement}
        id="compile"
        tooltipProps={{ className: 'keyboard-tooltip' }}
        overlayProps={{
          delay: { show: 500, hide: 0 },
          placement: detachRole === 'detached' ? 'bottom' : undefined,
        }}
      >
        <Button
          variant="primary"
          disabled={compiling}
          isLoading={compiling}
          onClick={() => startCompile()}
          className={buttonClassName}
          loadingLabel={`${t('compiling')}…`}
        >
          {t('recompile')}
        </Button>
      </Tooltip>

      <DropdownToggle
        as={DropdownToggleCustom}
        split
        variant="primary"
        id="pdf-recompile-dropdown"
        size="sm"
        aria-label={t('toggle_compile_options_menu')}
        className={dropdownToggleClassName}
      />

      <DropdownMenu>
        <DropdownHeader>{t('auto_compile')}</DropdownHeader>
        <li role="none">
          <DropdownItem as="button" onClick={() => setAutoCompile(true)} trailingIcon={autoCompile ? 'check' : null}>
            {t('on')}
          </DropdownItem>
        </li>
        <li role="none">
          <DropdownItem as="button" onClick={() => setAutoCompile(false)} trailingIcon={!autoCompile ? 'check' : null}>
            {t('off')}
          </DropdownItem>
        </li>
        <DropdownDivider />
        <DropdownHeader>{t('compile_mode')}</DropdownHeader>
        <li role="none">
          <DropdownItem as="button" onClick={() => setDraft(false)} trailingIcon={!draft ? 'check' : null}>
            {t('normal')}
          </DropdownItem>
        </li>
        <li role="none">
          <DropdownItem as="button" onClick={() => setDraft(true)} trailingIcon={draft ? 'check' : null}>
            {t('fast')}&nbsp;<span className="subdued">[draft]</span>
          </DropdownItem>
        </li>
        <DropdownDivider />
        <DropdownHeader>{t('syntax_checks')}</DropdownHeader>
        <li role="none">
          <DropdownItem
            as="button"
            onClick={() => setStopOnValidationError(true)}
            trailingIcon={stopOnValidationError ? 'check' : null}
          >
            {t('stop_on_validation_error')}
          </DropdownItem>
        </li>
        <li role="none">
          <DropdownItem
            as="button"
            onClick={() => setStopOnValidationError(false)}
            trailingIcon={!stopOnValidationError ? 'check' : null}
          >
            {t('ignore_validation_errors')}
          </DropdownItem>
        </li>
        <DropdownDivider />
        <DropdownHeader>{t('compile_error_handling')}</DropdownHeader>
        <li role="none">
          <DropdownItem as="button" onClick={enableStopOnFirstError} trailingIcon={stopOnFirstError ? 'check' : null}>
            {t('stop_on_first_error')}
          </DropdownItem>
        </li>
        <li role="none">
          <DropdownItem as="button" onClick={disableStopOnFirstError} trailingIcon={!stopOnFirstError ? 'check' : null}>
            {t('try_to_compile_despite_errors')}
          </DropdownItem>
        </li>
        <DropdownDivider />
        <li role="none">
          <DropdownItem as="button" onClick={() => stopCompile()} disabled={!compiling} aria-disabled={!compiling}>
            {t('stop_compile')}
          </DropdownItem>
        </li>
        <li role="none">
          <DropdownItem as="button" onClick={fromScratch} disabled={compiling} aria-disabled={compiling}>
            {t('recompile_from_scratch')}
          </DropdownItem>
        </li>
      </DropdownMenu>
    </Dropdown>
  )
}

export default memo(PdfCompileButton)
