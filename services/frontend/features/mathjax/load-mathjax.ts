declare global {
  interface Window {
    MathJax: Record<string, any>
  }
}

let mathJaxPromise: Promise<typeof window.MathJax>

/**
 * Loads MathJax, once, and hands back the global it defines.
 *
 * Every caller gets the same promise: MathJax configures itself from a global
 * object read at script load, so a second load with different options would
 * not reconfigure the first one -- it would fight with it. The options
 * therefore only take effect for whoever asks first, which in practice is the
 * preview, and the editor's widgets accept what it chose.
 */
export const loadMathJax = async (options?: {
  enableMenu?: boolean
  numbering?: string
  singleDollar?: boolean
  useLabelIds?: boolean
}) => {
  if (!mathJaxPromise) {
    mathJaxPromise = new Promise((resolve, reject) => {
      options = {
        enableMenu: false,
        singleDollar: true,
        useLabelIds: false,
        ...options,
      }

      const inlineMath = [['\\(', '\\)']]
      if (options.singleDollar) {
        inlineMath.push(['$', '$'])
      }

      // https://docs.mathjax.org/en/stable/options/index.html
      window.MathJax = {
        // https://docs.mathjax.org/en/latest/options/input/tex.html#the-configuration-block
        tex: {
          macros: {
            // Implements support for the \bm command from the bm package. It bolds the argument in math mode.
            // https://github.com/mathjax/MathJax/issues/1219#issuecomment-341059843
            bm: ['\\boldsymbol{#1}', 1],
          },
          inlineMath,
          displayMath: [
            ['\\[', '\\]'],
            ['$$', '$$'],
          ],
          packages: {
            '[-]': [
              'html', // avoid creating HTML elements/attributes
              'require', // prevent loading disabled packages
              'textmacros', // text macros are loaded by default in v4, disable them
            ],
          },
          processEscapes: true,
          processEnvironments: true,
          useLabelIds: options.useLabelIds,
        },
        output: {
          displayOverflow: 'linebreak',
        },
        loader: {
          load: [
            'ui/safe', // https://docs.mathjax.org/en/latest/options/safe.html
          ],
        },
        options: {
          enableMenu: options.enableMenu, // https://docs.mathjax.org/en/latest/options/menu.html
        },
        startup: {
          typeset: false,
          pageReady() {
            // disable the "Math Renderer" option in the context menu, as only SVG is available
            window.MathJax.startup.document.menu.menu
              .findID('Settings', 'Renderer')
              .disable()
          },
          async ready() {
            await window.MathJax.startup.defaultReady()

            // remove anything from the "font-family" attribute after a semicolon
            // so it's not added to the style attribute
            // https://github.com/mathjax/MathJax/issues/3129#issuecomment-1807225345
            const { safe } = window.MathJax.startup.document
            safe.filterAttributes.set('fontfamily', 'filterFontFamily')
            safe.filterMethods.filterFontFamily = (
              _safe: any,
              family: string
            ) => {
              return family.split(/;/)[0]
            }
          },
        },
      }

      // "tags" set separately as tags: undefined throws an error
      if (options.numbering) {
        window.MathJax.tex.tags = options.numbering
      }

      // if the menu is disabled, disable all the accessibility features, for performance
      // https://docs.mathjax.org/en/stable/options/accessibility.html#accessibility-extensions-options
      if (!options.enableMenu) {
        window.MathJax.options.menuOptions = {
          settings: {
            enrich: false,
            speech: false,
            braille: false,
            assistiveMml: false,
          },
        }
      }

      const script = document.createElement('script')
      // Written by next.config.ts from the installed package's version, and
      // matching where scripts/copy-mathjax.mjs put the files.
      const path = process.env.NEXT_PUBLIC_MATHJAX_PATH
      if (!path) {
        reject(new Error('No MathJax path found'))
        return
      }
      script.src = path
      script.addEventListener('load', async () => {
        await window.MathJax.startup.promise
        document.head.appendChild(window.MathJax.svgStylesheet())
        resolve(window.MathJax)
      })
      script.addEventListener('error', reject)
      document.head.append(script)
    })
  }

  return mathJaxPromise
}
