// Languages with a word list of their own shipped alongside the dictionary.
const dictionaryAdditions = new Set(['en_US'])

/**
 * The extra dictionary handed to Hunspell: this person's learned words, plus
 * the shipped additions for their language.
 *
 * Fetched rather than imported. The word lists are plain text put in place by
 * scripts/copy-hunspell.mjs; importing one would have the bundler try to read
 * it as a module, and a file of words is not a module.
 */
export const buildAdditionalDictionary = async (
  lang: string,
  learnedWords: string[]
) => {
  const words = [...learnedWords]

  if (dictionaryAdditions.has(lang)) {
    const response = await fetch(`/hunspell/wordlists/${lang}.txt`)
    if (response.ok) {
      const wordList = await response.text()
      words.push(...wordList.split('\n').filter(Boolean))
    }
  }

  // the first line contains the approximate word count
  words.unshift(String(words.length))

  return new TextEncoder().encode(words.join('\n'))
}
