import { heroui } from '@heroui/react'

/**
 * The theme.
 *
 * Tailwind v4 has no config file: what used to be `plugins: [heroui(...)]` is
 * a plugin loaded from the stylesheet with `@plugin`, and this is that plugin.
 *
 * Every value here comes from
 * services/web/frontend/stylesheets/foundations/tokens/*.json. HeroUI's own
 * palette is not used at all -- the point is to look like the thing this
 * replaces, and a component library's defaults are the fastest way not to.
 */

/** The primitives, so a scale is written once. */
const neutral = {
  10: '#f4f5f6',
  20: '#e7e9ee',
  30: '#d0d5dd',
  40: '#afb5c0',
  50: '#8d96a5',
  60: '#677283',
  70: '#495365',
  80: '#2f3a4c',
  90: '#1b222c',
}

const green = {
  10: '#eaf6ef',
  20: '#b8dbc8',
  30: '#86caa5',
  40: '#53b57f',
  50: '#098842',
  60: '#1e6b41',
  70: '#195936',
}

const blue = {
  10: '#f1f4f9',
  20: '#c3d0e3',
  30: '#97b6e5',
  40: '#6597e0',
  50: '#366cbf',
  60: '#28518f',
  70: '#214475',
}

const red = {
  10: '#f9f1f1',
  20: '#f5beba',
  30: '#e59d9a',
  40: '#e36d66',
  50: '#b83a33',
  60: '#942f2a',
  70: '#782722',
}

const yellow = {
  10: '#fcf1e3',
  20: '#fcc483',
  30: '#f7a445',
  40: '#de8014',
  50: '#8f5514',
  60: '#7a4304',
  70: '#633a0b',
}

/** HeroUI wants a 50..900 scale; these are the seven steps stretched over it. */
const scale = (steps: Record<number, string>, foreground: string) => ({
  50: steps[10],
  100: steps[10],
  200: steps[20],
  300: steps[30],
  400: steps[40],
  500: steps[50],
  600: steps[60],
  700: steps[70],
  800: steps[70],
  900: steps[70],
  DEFAULT: steps[50],
  foreground,
})

export default heroui({
  // 4px, which is border-radius-base. The original uses it for almost
  // everything; a pill is asked for explicitly where it is wanted.
  layout: {
    radius: { small: '4px', medium: '4px', large: '8px' },
    borderWidth: { small: '1px', medium: '1px', large: '2px' },
    fontSize: {
      tiny: '12px',
      small: '14px',
      medium: '16px',
      large: '18px',
    },
    lineHeight: {
      tiny: '16px',
      small: '20px',
      medium: '24px',
      large: '28px',
    },
  },
  themes: {
    light: {
      colors: {
        background: '#ffffff',
        foreground: neutral[90],
        divider: neutral[20],
        focus: blue[50],
        overlay: neutral[90],
        content1: { DEFAULT: '#ffffff', foreground: neutral[90] },
        content2: { DEFAULT: neutral[10], foreground: neutral[90] },
        content3: { DEFAULT: neutral[20], foreground: neutral[90] },
        content4: { DEFAULT: neutral[30], foreground: neutral[90] },
        default: {
          ...scale(neutral, neutral[90]),
          DEFAULT: neutral[20],
        },
        primary: scale(green, '#ffffff'),
        secondary: scale(blue, '#ffffff'),
        success: scale(green, '#ffffff'),
        warning: scale(yellow, '#ffffff'),
        danger: scale(red, '#ffffff'),
      },
    },
    dark: {
      colors: {
        background: neutral[90],
        foreground: '#ffffff',
        divider: neutral[80],
        focus: blue[40],
        overlay: '#000000',
        content1: { DEFAULT: neutral[80], foreground: '#ffffff' },
        content2: { DEFAULT: neutral[70], foreground: '#ffffff' },
        content3: { DEFAULT: neutral[60], foreground: '#ffffff' },
        content4: { DEFAULT: neutral[50], foreground: '#ffffff' },
        default: {
          50: neutral[90],
          100: neutral[80],
          200: neutral[70],
          300: neutral[60],
          400: neutral[50],
          500: neutral[40],
          600: neutral[30],
          700: neutral[20],
          800: neutral[10],
          900: '#ffffff',
          DEFAULT: neutral[70],
          foreground: '#ffffff',
        },
        primary: { ...scale(green, '#ffffff'), DEFAULT: green[40] },
        secondary: { ...scale(blue, '#ffffff'), DEFAULT: blue[40] },
        success: { ...scale(green, '#ffffff'), DEFAULT: green[40] },
        warning: { ...scale(yellow, neutral[90]), DEFAULT: yellow[30] },
        danger: { ...scale(red, '#ffffff'), DEFAULT: red[40] },
      },
    },
  },
})
