import logger from '@overleaf/logger'
import SocialAuthController from './SocialAuthController.mjs'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'
import { PROVIDERS } from './SocialAuthProviders.mjs'

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

    // The whitelist matches whole paths, so each provider's two addresses are
    // listed rather than the pattern they were mounted as. Without this a
    // site that requires a login sends somebody trying to sign in to the
    // sign-in page they were already on.
    AuthenticationController.addEndpointToLoginWhitelist('/auth/providers')
    for (const providerId of Object.keys(PROVIDERS)) {
      AuthenticationController.addEndpointToLoginWhitelist(`/auth/${providerId}`)
      AuthenticationController.addEndpointToLoginWhitelist(
        `/auth/${providerId}/callback`
      )
    }

    webRouter.post(
      '/user/oauth/:provider/unlink',
      AuthenticationController.requireLogin(),
      SocialAuthController.unlink
    )

    // What the account settings page posts to. It is shared with the other
    // authentication modules, so the handler passes on anything that is not a
    // provider of ours.
    webRouter.post(
      '/user/oauth-unlink',
      AuthenticationController.requireLogin(),
      SocialAuthController.unlinkFromSettings
    )
  },
}
