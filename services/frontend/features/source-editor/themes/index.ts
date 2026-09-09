/**
 * The editor colour themes, each loaded when first chosen.
 *
 * The JSON files hold a CodeMirror theme spec, a highlight style keyed by
 * the class names the class highlighter emits, and whether the theme is
 * dark. The list here doubles as the choice offered in the settings.
 */
export type ThemeSpec = {
  theme: Record<string, Record<string, unknown>>
  highlightStyle: Record<string, Record<string, unknown>>
  dark: boolean
}

type Loader = () => Promise<ThemeSpec>

const load = (loader: () => Promise<{ default: ThemeSpec } | ThemeSpec>): Loader => {
  return () => loader().then(module => ('default' in module ? module.default : module) as ThemeSpec)
}

export const editorThemes: Record<string, Loader> = {
  textmate: load(() => import('./cm6/textmate.json')),
  overleaf: load(() => import('./cm6/overleaf.json')),
  overleaf_dark: load(() => import('./cm6/overleaf_dark.json')),
  ambiance: load(() => import('./cm6/ambiance.json')),
  chaos: load(() => import('./cm6/chaos.json')),
  chrome: load(() => import('./cm6/chrome.json')),
  clouds: load(() => import('./cm6/clouds.json')),
  clouds_midnight: load(() => import('./cm6/clouds_midnight.json')),
  cobalt: load(() => import('./cm6/cobalt.json')),
  crimson_editor: load(() => import('./cm6/crimson_editor.json')),
  dawn: load(() => import('./cm6/dawn.json')),
  dracula: load(() => import('./cm6/dracula.json')),
  dreamweaver: load(() => import('./cm6/dreamweaver.json')),
  eclipse: load(() => import('./cm6/eclipse.json')),
  github: load(() => import('./cm6/github.json')),
  gob: load(() => import('./cm6/gob.json')),
  gruvbox: load(() => import('./cm6/gruvbox.json')),
  idle_fingers: load(() => import('./cm6/idle_fingers.json')),
  iplastic: load(() => import('./cm6/iplastic.json')),
  katzenmilch: load(() => import('./cm6/katzenmilch.json')),
  kr_theme: load(() => import('./cm6/kr_theme.json')),
  kuroir: load(() => import('./cm6/kuroir.json')),
  merbivore: load(() => import('./cm6/merbivore.json')),
  merbivore_soft: load(() => import('./cm6/merbivore_soft.json')),
  mono_industrial: load(() => import('./cm6/mono_industrial.json')),
  monokai: load(() => import('./cm6/monokai.json')),
  nord_dark: load(() => import('./cm6/nord_dark.json')),
  one_dark: load(() => import('./cm6/one_dark.json')),
  pastel_on_dark: load(() => import('./cm6/pastel_on_dark.json')),
  solarized_dark: load(() => import('./cm6/solarized_dark.json')),
  solarized_light: load(() => import('./cm6/solarized_light.json')),
  sqlserver: load(() => import('./cm6/sqlserver.json')),
  terminal: load(() => import('./cm6/terminal.json')),
  tomorrow: load(() => import('./cm6/tomorrow.json')),
  tomorrow_night: load(() => import('./cm6/tomorrow_night.json')),
  tomorrow_night_blue: load(() => import('./cm6/tomorrow_night_blue.json')),
  tomorrow_night_bright: load(() => import('./cm6/tomorrow_night_bright.json')),
  tomorrow_night_eighties: load(() => import('./cm6/tomorrow_night_eighties.json')),
  twilight: load(() => import('./cm6/twilight.json')),
  vibrant_ink: load(() => import('./cm6/vibrant_ink.json')),
  xcode: load(() => import('./cm6/xcode.json')),
}

/** The themes as the settings menu lists them, in the original's order. */
export const editorThemeNames = [
  { name: 'textmate', label: 'TextMate' },
  { name: 'overleaf', label: 'Overleaf' },
  { name: 'overleaf_dark', label: 'Overleaf Dark' },
  { name: 'ambiance', label: 'Ambiance' },
  { name: 'chaos', label: 'Chaos' },
  { name: 'chrome', label: 'Chrome' },
  { name: 'clouds', label: 'Clouds' },
  { name: 'clouds_midnight', label: 'Clouds Midnight' },
  { name: 'cobalt', label: 'Cobalt' },
  { name: 'crimson_editor', label: 'Crimson Editor' },
  { name: 'dawn', label: 'Dawn' },
  { name: 'dracula', label: 'Dracula' },
  { name: 'dreamweaver', label: 'Dreamweaver' },
  { name: 'eclipse', label: 'Eclipse' },
  { name: 'github', label: 'GitHub' },
  { name: 'gob', label: 'Gob' },
  { name: 'gruvbox', label: 'Gruvbox' },
  { name: 'idle_fingers', label: 'Idle Fingers' },
  { name: 'iplastic', label: 'IPlastic' },
  { name: 'katzenmilch', label: 'Katzenmilch' },
  { name: 'kr_theme', label: 'KR Theme' },
  { name: 'kuroir', label: 'Kuroir' },
  { name: 'merbivore', label: 'Merbivore' },
  { name: 'merbivore_soft', label: 'Merbivore Soft' },
  { name: 'mono_industrial', label: 'Mono Industrial' },
  { name: 'monokai', label: 'Monokai' },
  { name: 'nord_dark', label: 'Nord Dark' },
  { name: 'one_dark', label: 'One Dark' },
  { name: 'pastel_on_dark', label: 'Pastel on Dark' },
  { name: 'solarized_dark', label: 'Solarized Dark' },
  { name: 'solarized_light', label: 'Solarized Light' },
  { name: 'sqlserver', label: 'SQL Server' },
  { name: 'terminal', label: 'Terminal' },
  { name: 'tomorrow', label: 'Tomorrow' },
  { name: 'tomorrow_night', label: 'Tomorrow Night' },
  { name: 'tomorrow_night_blue', label: 'Tomorrow Night Blue' },
  { name: 'tomorrow_night_bright', label: 'Tomorrow Night Bright' },
  { name: 'tomorrow_night_eighties', label: 'Tomorrow Night Eighties' },
  { name: 'twilight', label: 'Twilight' },
  { name: 'vibrant_ink', label: 'Vibrant Ink' },
  { name: 'xcode', label: 'Xcode' },
]
