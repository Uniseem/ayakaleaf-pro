'use client'

/**
 * The small shared pieces: copy to clipboard, the close cross, and the
 * keyboard shortcut label, from shared/components.
 */

import { memo, useCallback, useState } from 'react'
import { Button } from './button'
import { IconButton } from './icon-button'
import MaterialIcon from './material-icon'
import { Tooltip } from './tooltip'
import { useTranslation } from '@/lib/i18n'

export const CopyToClipboard = memo(function CopyToClipboard({
  content,
  tooltipId,
  kind = 'icon',
  unfilled = false,
  onClick,
}: {
  content: string
  tooltipId: string
  kind?: 'text' | 'icon' | 'button'
  unfilled?: boolean
  onClick?: () => void
}) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleClick = useCallback(() => {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
    onClick?.()
  }, [content, onClick])

  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    return null
  }

  return (
    <Tooltip id={tooltipId} description={copied ? `${t('copied')}!` : t('copy')} overlayProps={{ delay: copied ? 1000 : 250 }}>
      {kind === 'text' ? (
        <Button onClick={handleClick} size="sm" variant="secondary" className="copy-button">
          {t('copy')}
        </Button>
      ) : kind === 'button' ? (
        <Button onClick={handleClick} size="sm" variant="ghost" className="copy-button copy-button-ghost">
          {copied ? <MaterialIcon type="check" /> : <MaterialIcon type="content_copy" unfilled />}
          {t('copy')}
        </Button>
      ) : (
        <IconButton
          onClick={handleClick}
          variant="link"
          size="sm"
          accessibilityLabel={t('copy')}
          className="copy-button"
          icon={copied ? 'check' : 'content_copy'}
          unfilled={unfilled as false}
        />
      )}
    </Tooltip>
  )
})

export function Close({
  onDismiss,
  variant = 'light',
}: {
  onDismiss: React.MouseEventHandler<HTMLButtonElement>
  variant?: 'light' | 'dark'
}) {
  const { t } = useTranslation()
  return (
    <button type="button" className={`close float-end ${variant}`} onClick={onDismiss}>
      <MaterialIcon type="close" className="align-text-bottom" accessibilityLabel={t('close')} />
    </button>
  )
}

export function Shortcut({ keys }: { keys: string[] }) {
  return (
    <span>
      {keys.map((key, index) => (
        <span className={key.length === 1 ? 'dropdown-shortcut-char' : undefined} key={`${key}${index}`}>
          {key}
        </span>
      ))}
    </span>
  )
}
