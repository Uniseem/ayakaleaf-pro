import type { Config } from 'tailwindcss'
import { heroui } from '@heroui/react'

// Nothing here has to coexist with Bootstrap: this app shares no stylesheet
// with the client it replaces, so preflight stays on and no class prefix is
// needed. That is most of what made adding Tailwind to the old frontend
// expensive, and none of it applies to a tree that starts empty.
const config: Config = {
  content: [
    './app/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
    './features/**/*.{ts,tsx}',
    './node_modules/@heroui/theme/dist/**/*.{js,ts,jsx,tsx}',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-sans)', 'system-ui', 'sans-serif'],
        mono: ['var(--font-mono)', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
    },
  },
  plugins: [
    heroui({
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
    }),
  ],
}

export default config
