import { useEffect, useState } from 'react'
import { HunspellManager } from '@/features/source-editor/hunspell/HunspellManager'
import { globalIgnoredWords } from '@/features/dictionary/ignored-words'
import {
  learnedWords,
  setLearnedWords,
} from '@/features/source-editor/extensions/spelling/learned-words'
import { learnedWords as fetchLearnedWords } from '@/lib/settings'
import { spellCheckLanguages } from '@/lib/spell-check-languages'
import { debugConsole } from '@/lib/debug'

const supportsWebAssembly = () => typeof window.WebAssembly === 'object'

/**
 * A spell checker for the chosen language, or nothing.
 *
 * Nothing when the language has no dictionary shipped with the client, or
 * when the browser has no WebAssembly: the editor then simply underlines
 * nothing, which is what the original does in the same situations.
 */
export const useHunspell = (spellCheckLanguage: string | null) => {
  const [hunspellManager, setHunspellManager] = useState<HunspellManager>()

  // The personal dictionary is fetched once and kept, because the worker is
  // handed the whole list when it starts: a word learned later is added to the
  // running worker as well, so this only has to be right at the beginning.
  const [dictionaryLoaded, setDictionaryLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    fetchLearnedWords()
      .then(words => {
        if (!cancelled) {
          setLearnedWords(words)
        }
      })
      .catch(debugConsole.error)
      .finally(() => {
        // Even a failure lets the spell checker start: it then knows nothing
        // of this person's own words, which is worse than knowing them and
        // much better than no spell check at all.
        if (!cancelled) {
          setDictionaryLoaded(true)
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (spellCheckLanguage && dictionaryLoaded && supportsWebAssembly()) {
      const lang = spellCheckLanguages.find(item => item.code === spellCheckLanguage)
      if (lang?.dic) {
        const hunspellManager = new HunspellManager(lang.dic, [
          ...globalIgnoredWords,
          ...learnedWords.global,
        ])
        setHunspellManager(hunspellManager)
        debugConsole.log(spellCheckLanguage, hunspellManager)

        return () => {
          hunspellManager.destroy()
        }
      } else {
        setHunspellManager(undefined)
      }
    }
  }, [spellCheckLanguage, dictionaryLoaded])

  return hunspellManager
}
