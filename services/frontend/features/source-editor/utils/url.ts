const ALLOWED_PROTOCOLS = ['https:', 'http:']

/**
 * Opens a URL taken from the document.
 *
 * The protocol is checked because the string comes from the document, and a
 * document can be shared: javascript: and data: URLs would run in this origin
 * if a link tooltip opened them.
 */
export const openURL = (content: string) => {
  const url = new URL(content, document.location.href)

  if (!ALLOWED_PROTOCOLS.includes(url.protocol)) {
    throw new Error(`Not opening URL with protocol ${url.protocol}`)
  }

  window.open(url, '_blank')
}
