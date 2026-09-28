import router from './app/src/SocialAuthRouter.mjs'
import { enabledProviders } from './app/src/SocialAuthProviders.mjs'

// Signing in with Google or GitHub, and attaching either to an account that
// already exists. Which providers are offered comes from the settings store,
// so this module is always loaded and offers nothing until one is configured.

export default {
  name: 'social-auth',
  router,
  enabledProviders,
}
