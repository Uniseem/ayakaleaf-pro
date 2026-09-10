/**
 * A Material Symbols icon as a plain element.
 *
 * The React component cannot be used everywhere: CodeMirror widgets build
 * their own DOM outside of React's tree, and this is the same icon for them.
 */
export function materialIcon(type: string) {
  const icon = document.createElement('span')
  icon.className = 'material-symbols'
  icon.textContent = type
  icon.ariaHidden = 'true'
  icon.translate = false

  return icon
}
