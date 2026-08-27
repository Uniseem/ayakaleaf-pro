import logger from '@overleaf/logger'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'
import OpenInOverleafController from './OpenInOverleafController.mjs'

// "Open in Overleaf" / Prefilled-Project API (documented at /devs). Reproduces
// the SaaS-only `/docs` endpoint on Ayakaleaf Pro.
//
//   GET  /docs   — link form (`/docs?snip_uri=…` / `?encoded_snip=…`). No Origin
//                  header on a top-level navigation, so it is never blocked by
//                  blockCrossOriginRequests, and requireLogin's postLoginRedirect
//                  carries the query params through a login round-trip.
//   POST /docs   — form/AJAX form. CSRF is disabled here (external submitters
//                  have no token). Cross-origin POSTs still need the submitting
//                  site's origin in Settings.allowedOrigins; same-origin (the
//                  /devs examples) always works.
//   GET  /devs   — public "Overleaf API" documentation page.
export default {
  apply(webRouter) {
    logger.debug({}, 'Init open-in-overleaf router')

    // External sites post here without a CSRF token; exempt the route (the
    // csurf middleware still attaches req.csrfToken() for rendered forms).
    if (webRouter.csrf) {
      webRouter.csrf.disableDefaultCsrfProtection('/docs', 'POST')
    }

    webRouter.get(
      '/docs',
      AuthenticationController.requireLogin(),
      OpenInOverleafController.openInOverleaf
    )
    webRouter.post(
      '/docs',
      AuthenticationController.requireLogin(),
      OpenInOverleafController.openInOverleaf
    )

    webRouter.get('/devs', OpenInOverleafController.devsPage)
  },
}
