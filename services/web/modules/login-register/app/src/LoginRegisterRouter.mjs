import logger from '@overleaf/logger'
import Settings from '@overleaf/settings'
import RegisterController from './RegisterController.mjs'
import LoginController from './LoginController.mjs'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'
import RateLimiterMiddleware from '../../../../app/src/Features/Security/RateLimiterMiddleware.mjs'
import { RateLimiter } from '../../../../app/src/infrastructure/RateLimiter.mjs'

// Limit registration attempts per IP. This is the only defence left once the
// routes exist whether or not registration is open, so it matters more than it
// did when a closed site simply had no /register.
const registrationRateLimiters = {
  postRegister: new RateLimiter('postRegister', {
    points: 5,
    duration: 60,
  }),
}

export default {
  apply(webRouter) {
    logger.debug({}, 'Init Registration module')

    // The routes exist whatever the current policy says, and refuse in the
    // handler. Registering them conditionally would mean an administrator who
    // turns sign-up on in the admin page has to restart the site before
    // anybody can use it.
    webRouter.stack = webRouter.stack.filter(layer => {
      return !(
        layer.route &&
        layer.route.path === '/register' &&
        layer.route.methods.get
      )
    })

    webRouter.get('/register', RegisterController.registerPage)
    webRouter.get('/register/status', RegisterController.registrationStatus)
    AuthenticationController.addEndpointToLoginWhitelist('/register')
    AuthenticationController.addEndpointToLoginWhitelist('/register/status')

    // Signing up with a password is what the page offers. The second route
    // sends somebody an activation link instead, which needs a mail server and
    // so is left to an administrator or a script rather than put on the page;
    // both ask the policy the same question.
    webRouter.post(
      '/register',
      RateLimiterMiddleware.rateLimit(registrationRateLimiters.postRegister),
      RegisterController.registerWithUsernameAndPassword
    )
    webRouter.post(
      '/register/email',
      RateLimiterMiddleware.rateLimit(registrationRateLimiters.postRegister),
      RegisterController.registerWithEmail
    )
    AuthenticationController.addEndpointToLoginWhitelist('/register/email')

    // Public login
    webRouter.stack = webRouter.stack.filter(layer => {
      return !(
        layer.route &&
        layer.route.path === '/login' &&
        layer.route.methods.get
      )
    })

    webRouter.get('/login', LoginController.loginPage)
    AuthenticationController.addEndpointToLoginWhitelist('/login')

    if (Settings.ldap && Settings.ldap.enable) {
      webRouter.get('/ldap/login', LoginController.ldapLoginPage)
      AuthenticationController.addEndpointToLoginWhitelist('/ldap/login')
    }
  },
}
