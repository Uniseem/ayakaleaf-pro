import AuthenticationController from "../../../../app/src/Features/Authentication/AuthenticationController.mjs"
import Settings from "@overleaf/settings"
import Path from 'path'
import RegistrationPolicy from './RegistrationPolicy.mjs'

export default {
  async loginPage(req, res, next) {
    // A site nobody has set up yet has nothing to sign in to. Sending the
    // first visitor to the sign-up form is what makes a fresh deployment
    // claimable by reaching it, rather than by finding a token in a log.
    if (await RegistrationPolicy.noAdminExists()) {
      return res.redirect('/register')
    }
    // return res.json({ message: 'Login successful' })
    // if user is being sent to /login with explicit redirect (redir=/foo),
    // such as being sent from the editor to /login, then set the redirect explicitly
    if (
      req.query.redir != null &&
      AuthenticationController.getRedirectFromSession(req) == null
    ) {
      AuthenticationController.setRedirectInSession(req, req.query.redir)
    }
    const metadata = { robotsNoindexNofollow: false }
    if (Object.keys(req.query).length !== 0) {
      metadata.robotsNoindexNofollow = true
    }
    // Why a Google or GitHub sign-in did not go through, shown once. It is
    // carried in the session rather than the query string so that nobody can
    // put words on this page with a link.
    const socialAuthError = req.session.socialAuthError
    delete req.session.socialAuthError

    const __dirname = Path.dirname(new URL(import.meta.url).pathname)
    res.render(Path.resolve(__dirname, '../views/user/login'), {
      title: Settings.nav?.login_support_title || 'login',
      login_support_title: Settings.nav?.login_support_title,
      login_support_text: Settings.nav?.login_support_text,
      socialAuthError,
      metadata,
    })
  },

  async ldapLoginPage(req, res, next) {
    // return res.json({ message: 'Login successful' })
    // if user is being sent to /login with explicit redirect (redir=/foo),
    // such as being sent from the editor to /login, then set the redirect explicitly
    if (
      req.query.redir != null &&
      AuthenticationController.getRedirectFromSession(req) == null
    ) {
      AuthenticationController.setRedirectInSession(req, req.query.redir)
    }
    const metadata = { robotsNoindexNofollow: false }
    if (Object.keys(req.query).length !== 0) {
      metadata.robotsNoindexNofollow = true
    }
    const __dirname = Path.dirname(new URL(import.meta.url).pathname)
    res.render(Path.resolve(__dirname, '../views/user/ldap-login'), {
      title: Settings.nav?.login_support_title || 'login',
      login_support_title: Settings.nav?.login_support_title,
      login_support_text: Settings.nav?.login_support_text,
      metadata,
    })
  }
}