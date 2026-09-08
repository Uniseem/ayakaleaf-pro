/**
 * A path this site will honour after signing in.
 *
 * Only a path here: a value that arrived in the address bar and could name
 * another site would turn the sign-in page into a way of sending somebody
 * somewhere else under our name. A leading double slash is a URL, not a path.
 */
export function localPath(next: string | undefined, fallback: string): string {
  if (!next || !next.startsWith('/') || next.startsWith('//')) {
    return fallback
  }
  return next
}
