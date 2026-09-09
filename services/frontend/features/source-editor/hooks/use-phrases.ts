import { useMemo } from 'react'
import { useTranslation, exists } from '@/lib/i18n'

/**
 * The strings CodeMirror's own panels and messages use, translated.
 *
 * A proxy rather than a copy of every string: CodeMirror asks for a phrase
 * by its English text, and only the ones it asks for are looked up.
 */
export const usePhrases = (): Record<string, string> => {
  const { t } = useTranslation()

  const codemirrorBuiltinsOverrides = useMemo(
    () => ({
      'Fold line': t('fold_line'),
      'Unfold line': t('unfold_line'),
    }),
    [t]
  )

  const translationProxy = useMemo(
    () => ({
      getOwnPropertyDescriptor(target: Record<string, string>, prop: string) {
        // An override that was added
        if (Object.prototype.hasOwnProperty.call(target, prop)) {
          return Object.getOwnPropertyDescriptor(target, prop)
        }
        // A translation that exists is reported as a property:
        //   non-enumerable: it won't show up when enumerating the keys of the target
        //   configurable: it has to be reported as configurable since it doesn't
        //                 exist in the base object
        //   writable: an override can be added
        if (exists(prop)) {
          return { enumerable: false, configurable: true, writable: true }
        }
        return Object.getOwnPropertyDescriptor(target, prop)
      },
      get(target: Record<string, string>, prop: string) {
        // An override that was added
        if (Object.prototype.hasOwnProperty.call(target, prop)) {
          return target[prop]
        }
        if (exists(prop)) {
          return t(prop)
        }
        return target[prop]
      },
    }),
    [t]
  )

  const phrases = useMemo(
    () => new Proxy(codemirrorBuiltinsOverrides, translationProxy) as Record<string, string>,
    [translationProxy, codemirrorBuiltinsOverrides]
  )

  return phrases
}
