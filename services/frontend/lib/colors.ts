/**
 * The colour each collaborator is drawn in, from shared/utils/colors.ts.
 *
 * A hue from a hash of the user's id, so the same person is the same colour
 * in every project and on every screen; one's own hue is fixed at 200 and
 * kept clear of everybody else's, so "me" is always recognisable.
 */

import { md5 } from './md5'

const ANONYMOUS_HUE = 100
const OWN_HUE = 200
const OWN_HUE_BLOCKED_SIZE = 20
const TOTAL_HUES = 360

let ownUserId: string | undefined

/** Tells this module who "me" is. */
export function setOwnUserId(id: string | undefined) {
  ownUserId = id
}

export function getHueForUserId(userId?: string): number {
  if (userId == null || userId === 'anonymous-user') {
    return ANONYMOUS_HUE
  }

  if (ownUserId === userId) {
    return OWN_HUE
  }

  let hue = getHueForId(userId)

  if (hue > OWN_HUE - OWN_HUE_BLOCKED_SIZE && hue < OWN_HUE + OWN_HUE_BLOCKED_SIZE) {
    hue = hue - OWN_HUE
    hue = hue + TOTAL_HUES - OWN_HUE_BLOCKED_SIZE
  }

  return hue
}

export function getBackgroundColorForUserId(userId?: string) {
  return `hsl(${getHueForUserId(userId)}, 70%, 50%)`
}

export function hslStringToLuminance(hslString: string): number {
  const hslSplit = hslString.slice(4).split(')')[0]!.split(',')

  const h = Number(hslSplit[0])
  const s = Number(hslSplit[1]!.slice(0, -1)) / 100
  const l = Number(hslSplit[2]!.slice(0, -1)) / 100

  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  let r = 0
  let g = 0
  let b = 0
  if (h >= 0 && h < 60) {
    r = c + m
    g = x + m
    b = m
  } else if (h >= 60 && h < 120) {
    r = x + m
    g = c + m
    b = m
  } else if (h >= 120 && h < 180) {
    r = m
    g = c + m
    b = x + m
  } else if (h >= 180 && h < 240) {
    r = m
    g = x + m
    b = c + m
  } else if (h >= 240 && h < 300) {
    r = x + m
    g = m
    b = c + m
  } else if (h >= 300 && h < 360) {
    r = c + m
    g = m
    b = x + m
  }

  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const cachedHues = new Map<string, number>()

export function getHueForId(id: string) {
  const cached = cachedHues.get(id)
  if (cached !== undefined) {
    return cached
  }

  const hash = md5(id)
  const hue = parseInt(hash.slice(0, 8), 16) % (TOTAL_HUES - OWN_HUE_BLOCKED_SIZE * 2)

  cachedHues.set(id, hue)

  return hue
}

/** The first grapheme of a string, for the initial in a circle. */
export function firstCharacter(str: string): string {
  if (!str) {
    return ''
  }

  if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
    try {
      const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
      for (const { segment } of segmenter.segment(str)) {
        return segment
      }
    } catch {
      // fall through
    }
  }

  const [first] = [...str]
  return first ?? ''
}
