'use client'

/**
 * The strings on screen.
 *
 * The original refers to every string by a key into locales/<lang>.json, and
 * this client uses the same keys and the same file (a subset of it, built by
 * scripts/extract-translations.mjs), so the words are the same words.
 *
 * The format is i18next's older one, which is what those files are written
 * in: `__name__` for a value, `key_plural` for a count other than one, and
 * `<0>...</0>` or `<b>...</b>` for a component wrapped around part of the
 * text. All three are handled here, without the library, because the client
 * only ever needs to read them.
 */

import { Fragment, cloneElement, createElement, isValidElement, type ReactNode } from 'react'
import en from './locales/en.json'

type Values = Record<string, string | number | undefined>

const strings: Record<string, string> = en

/** The raw string for a key, with the plural form chosen by `count`. */
function lookup(key: string, values?: Values): string {
  const count = values?.count
  if (typeof count === 'number' && count !== 1) {
    const plural = strings[`${key}_plural`]
    if (plural !== undefined) {
      return plural
    }
  }
  const found = strings[key]
  if (found === undefined) {
    if (process.env.NODE_ENV !== 'production') {
      console.warn(`[i18n] missing string: ${key}`)
    }
    return key
  }
  return found
}

function interpolate(text: string, values?: Values): string {
  if (!values) {
    return text
  }
  return text.replace(/__([a-zA-Z0-9]+)__/g, (whole, name: string) => {
    const value = values[name]
    return value === undefined ? whole : String(value)
  })
}

/** The string for a key, with values filled in. */
export function t(key: string, values?: Values): string {
  return interpolate(lookup(key, values), values)
}

/** Reads like the original's hook, so components look the same. */
export function useTranslation() {
  return { t }
}

type TransProps = {
  i18nKey: string
  values?: Values
  /** Elements for `<0>`, `<1>`, ... in the string, by index. */
  components?: ReactNode[] | Record<string, ReactNode>
  /** Whether `&amp;`-style escapes in the string should be undone. */
  shouldUnescape?: boolean
}

const entities: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&#x27;': "'",
}

function unescape(text: string): string {
  return text.replace(/&(amp|lt|gt|quot|#39|#x27);/g, match => entities[match] ?? match)
}

/**
 * A string with elements inside it.
 *
 * `<0>text</0>` wraps `text` in components[0]; `<b>text</b>` wraps it in
 * components.b, or in a plain <b> when none is given. Tags nest.
 */
export function Trans({ i18nKey, values, components, shouldUnescape }: TransProps) {
  let text = interpolate(lookup(i18nKey, values), values)
  if (shouldUnescape) {
    text = unescape(text)
  }
  return <>{render(text, components)}</>
}

function componentFor(
  name: string,
  components?: TransProps['components']
): ReactNode | undefined {
  if (!components) {
    return undefined
  }
  if (Array.isArray(components)) {
    const index = Number(name)
    return Number.isInteger(index) ? components[index] : undefined
  }
  return components[name]
}

function render(text: string, components?: TransProps['components']): ReactNode[] {
  const out: ReactNode[] = []
  const tag = /<([a-zA-Z0-9]+)>([\s\S]*?)<\/\1>/g
  let last = 0
  let key = 0
  for (const match of text.matchAll(tag)) {
    const [whole, name, inner] = match
    const at = match.index ?? 0
    if (at > last) {
      out.push(text.slice(last, at))
    }
    const children = render(inner ?? '', components)
    const element = componentFor(name ?? '', components)
    if (isValidElement(element)) {
      out.push(cloneElement(element, { key: key++ }, ...children))
    } else if (name && /^[a-z]+$/.test(name)) {
      out.push(createElement(name, { key: key++ }, ...children))
    } else {
      out.push(<Fragment key={key++}>{children}</Fragment>)
    }
    last = at + whole.length
  }
  if (last < text.length) {
    out.push(text.slice(last))
  }
  return out
}
