/**
 * Who made a tracked change, from review-panel/context/changes-users-context.
 *
 * The ranges carry a user id and nothing else, so the name has to come from
 * somewhere the client already knows about people: everybody currently in the
 * project, which covers the case that matters, and a colour derived from the
 * id, which is the same colour their cursor has.
 */

import { getHueForUserId } from '@/lib/colors'

const names = new Map<string, string>()

/** Told by the connection context as people arrive. */
export function rememberUser(id: string, name: string) {
  if (id && name) {
    names.set(id, name)
  }
}

export function nameFor(id?: string): string | undefined {
  return id ? names.get(id) : undefined
}

export function hueFor(id?: string): number {
  return getHueForUserId(id ?? '')
}
