import { promisify } from 'node:util'

// Local logout for SSO flows via the public passport/session primitives
// (core UserController.doLogout is private and not exported).
export async function endSession(req) {
  if (typeof req.logout === 'function') {
    await promisify(req.logout.bind(req))()
  }
  await promisify(req.session.destroy.bind(req.session))()
}

// logout is a router dispatcher that will call the appropriate
// SSO logout flow if the user is logged in via SSO,
// otherwise it calls next() to continue local logout flow.
export default async function logout(req, res, next) {
  if (req.user && req.user.externalAuth) {
    switch (req.user.externalAuth) {
      case 'saml': {
        const { default: SAMLAuthenticationController } = await import(
          './saml/app/src/SAMLAuthenticationController.mjs'
        )
        return SAMLAuthenticationController.passportLogout(req, res, next)
      }
      case 'oidc': {
        const { default: OIDCAuthenticationController } = await import(
          './oidc/app/src/OIDCAuthenticationController.mjs'
        )
        return OIDCAuthenticationController.passportLogout(req, res, next)
      }
      default:
        next()
    }
  } else {
    next()
  }
}
