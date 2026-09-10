import { useEditorPropertiesContext } from '@/features/ide/contexts/editor-properties-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useTranslation } from '@/lib/i18n'

/**
 * Closes the palette and hands focus back to the editor.
 *
 * The icon is a dark image on a light ground, so under the dark theme it is
 * inverted rather than swapped for a second asset.
 */
export function SymbolPaletteCloseButton() {
  const { toggleSymbolPalette } = useEditorPropertiesContext()
  const { t } = useTranslation()
  const { activeOverallTheme } = useCompile()

  const handleClick = () => {
    toggleSymbolPalette()
    window.dispatchEvent(new Event('editor:focus'))
  }

  return (
    <div className="symbol-palette-close-button-outer">
      <button
        type="button"
        className="btn-close symbol-palette-close-button"
        style={
          activeOverallTheme === 'dark'
            ? { filter: 'invert(1) grayscale(100%) brightness(200%)' }
            : undefined
        }
        onClick={handleClick}
        aria-label={t('close')}
      />
    </div>
  )
}
