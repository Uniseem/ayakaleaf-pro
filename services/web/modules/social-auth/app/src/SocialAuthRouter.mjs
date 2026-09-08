import logger from '@overleaf/logger'
import SocialAuthController from './SocialAuthController.mjs'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'

export default {
  apply(webRouter) {
    logger.debug({}, 'Init social auth router')

    // Mounted whether or not a provider is configured; the handler answers 404
    // for one that is not, so that turning a provider on in the admin page
    // needs no restart.
    // Before the :provider route, which would otherwise swallow it.
    webRouter.get('/auth/providers', SocialAuthController.status)
    webRouter.get('/auth/:provider', SocialAuthController.start)
    webRouter.get('/auth/:provider/callback', SocialAuthController.callback)

    AuthenticationController.addEndpointToLoginWhitelist('/auth/providers')

    webRouter.post(
      '/user/oauth/:provider/unlink',
      AuthenticationController.requireLogin(),
      SocialAuthController.unlink
    )
  },
}
