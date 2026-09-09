import { keymap } from '@codemirror/view'
import { defaultKeymap, historyKeymap } from '@codemirror/commands'
import { lintKeymap } from '@codemirror/lint'
import { scrollOneLineKeymap } from './scroll-one-line'
import { foldingKeymap } from './folding-keymap'

const ignoredDefaultKeybindings = new Set([
  // NOTE: disable "Mod-Enter" as it's used for "Compile"
  'Mod-Enter',
  // Disable Alt+Arrow as there is special behaviour on Windows / Linux
  'Alt-ArrowLeft',
  'Alt-ArrowRight',
  // This keybinding causes issues on some keyboard layouts where \ is entered
  // using AltGr. Windows treats Ctrl-Alt as AltGr, so trying to insert a \
  // with Ctrl-Alt would trigger this keybinding, rather than inserting a \
  'Mod-Alt-\\',
])

const ignoredDefaultMacKeybindings = new Set([
  // These are replaced with custom visual-line versions
  'Mod-Backspace',
  'Mod-Delete',
  // Disable toggleTabFocusMode as it conflicts with ” on a Swedish keyboard layout
  'Shift-Alt-m',
])

const filteredDefaultKeymap = defaultKeymap.filter(
  // Only filter on keys, so a keybinding without a key is allowed
  item => {
    if (item.key && ignoredDefaultKeybindings.has(item.key)) {
      return false
    }
    if (item.mac && ignoredDefaultMacKeybindings.has(item.mac)) {
      return false
    }
    return true
  }
)

export const keymaps = keymap.of([
  // The default CodeMirror keymap, with a few key bindings filtered out.
  ...filteredDefaultKeymap,
  // Key bindings for undo/redo/undoSelection/redoSelection
  ...historyKeymap,
  // Key bindings for "open lint panel" and "next diagnostic"
  ...lintKeymap,
  // Key bindings for folding actions
  ...foldingKeymap,
  // Key bindings for scrolling the viewport
  ...scrollOneLineKeymap,
])
