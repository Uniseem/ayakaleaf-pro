import { heroui } from '@heroui/react'

/**
 * The theme.
 *
 * Tailwind v4 has no config file: what used to be `plugins: [heroui(...)]` is
 * a plugin loaded from the stylesheet with `@plugin`, and this is that plugin.
 * Only the colours that are ours are set; everything else is HeroUI's, which
 * is the point of using it.
 */
export default heroui({
  themes: {
    light: {
      colors: {
        primary: {
          DEFAULT: '#138a36',
          foreground: '#ffffff',
        },
        focus: '#138a36',
      },
    },
    dark: {
      colors: {
        primary: {
          DEFAULT: '#3fb15f',
          foreground: '#08130c',
        },
        focus: '#3fb15f',
      },
    },
  },
})
