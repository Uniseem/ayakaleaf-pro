import { StateEffect } from '@codemirror/state'

export const addLearnedWordEffect = StateEffect.define<string>()

export const removeLearnedWordEffect = StateEffect.define<string>()

/**
 * The words this person has told the spell checker to accept.
 *
 * A module-level set rather than state, because the worker is handed the list
 * once when it starts and the extension needs to answer "is this word known"
 * synchronously while decorating. It is filled from the API by
 * setLearnedWords once they have been fetched.
 */
export const learnedWords = {
  global: new Set<string>(),
}

export const setLearnedWords = (words: Iterable<string>) => {
  learnedWords.global = new Set(words)
}

export const addLearnedWord = (text: string) => {
  learnedWords.global.add(text)
  return {
    effects: addLearnedWordEffect.of(text),
  }
}

export const removeLearnedWord = (text: string) => {
  learnedWords.global.delete(text)
  return {
    effects: removeLearnedWordEffect.of(text),
  }
}
