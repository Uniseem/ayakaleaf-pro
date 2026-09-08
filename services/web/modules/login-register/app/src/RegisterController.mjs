import Path from 'path'
import logger from '@overleaf/logger'
import Settings from '@overleaf/settings'
import { expressify } from '@overleaf/promise-utils'
import UserRegistrationHandler from '../../../../app/src/Features/User/UserRegistrationHandler.mjs'
import AuthenticationManager from '../../../../app/src/Features/Authentication/AuthenticationManager.mjs'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'
import HaveIBeenPwned from '../../../../app/src/Features/Authentication/HaveIBeenPwned.mjs'
import RegistrationPolicy from './RegistrationPolicy.mjs'

// Registration is offered or refused when somebody asks, not when the process
// started: whether it is open now comes from the admin page.
//
// The first account on a site is a special case in one way only -- it is
// allowed however registration is configured, and it becomes the
// administrator. That is what makes a fresh deployment claimable by whoever
// reaches it first, which is the same trust model as the token the installer
// used to print, without the token.

async function registerPage(req, res) {
  if (req.user != null) {
    return res.redirect('/')
  }
  const policy = await RegistrationPolicy.describe()

  const __dirname = Path.dirname(new URL(import.meta.url).pathname)
  res.render(Path.resolve(__dirname, '../views/user/register'), {
    showPasswordField: true,
    isFirstUser: policy.isFirstUser,
    canRegister: policy.canRegister,
    allowedDomains: policy.allowedDomains,
    oauthProviders: Settings.oauthProviders || {},
    csrfToken: req.csrfToken(),
  })
}

/** What the sign-up page needs, for anything that is not that page. */
async function registrationStatus(req, res) {
  res.json(await RegistrationPolicy.describe())
}

async function registerWithUsernameAndPassword(req, res, next) {
  const { email, password } = req.body
  if (!email || !password) {
    return res.status(422).json({ message: 'an email and a password are required' })
  }

  const invalidEmail = AuthenticationManager.validateEmail(email)
  if (invalidEmail) {
    return res
      .status(400)
      .json({ message: { type: 'error', text: invalidEmail.message } })
  }

  const decision = await RegistrationPolicy.mayRegister(email)
  if (!decision.allowed) {
    return res.status(403).json({ message: { type: 'error', text: decision.reason } })
  }

  const invalidPassword = AuthenticationManager.validatePassword(password, email)
  if (invalidPassword) {
    return res
      .status(400)
      .json({ message: { type: 'error', text: invalidPassword.message } })
  }

  let isPasswordReused
  try {
    isPasswordReused =
      await HaveIBeenPwned.promises.checkPasswordForReuse(password)
  } catch (error) {
    logger.debug({ err: error }, 'could not check the password against HaveIBeenPwned')
  }
  if (isPasswordReused) {
    return res.status(400).json({
      message: {
        type: 'error',
        key: 'password-must-be-strong',
        text: 'This password has been seen in a data breach and cannot be used. Please choose a different one.',
      },
    })
  }

  let user
  try {
    user = await UserRegistrationHandler.promises.registerNewUser({
      email,
      password,
    })
  } catch (error) {
    logger.debug({ err: error }, 'could not register the user')
    return res.status(400).json({ message: error.message })
  }

  if (decision.isFirstUser) {
    await RegistrationPolicy.promoteIfFirstUser(user._id)
  }

  // Signed in rather than sent back to the login page to type the same
  // password again: somebody who just chose it is holding the session that
  // chose it. Core sends people to /login here because registration there is
  // an administrator creating an account for somebody else, which is not what
  // this is.
  return AuthenticationController.finishLogin(user, req, res, next)
}

async function registerWithEmail(req, res, next) {
  const { email } = req.body
  if (!email) {
    return res.status(422).json({ message: 'an email is required' })
  }

  const invalidEmail = AuthenticationManager.validateEmail(email)
  if (invalidEmail) {
    return res
      .status(400)
      .json({ message: { type: 'error', text: invalidEmail.message } })
  }

  const decision = await RegistrationPolicy.mayRegister(email)
  if (!decision.allowed) {
    return res.status(403).json({ message: { type: 'error', text: decision.reason } })
  }

  await UserRegistrationHandler.promises.registerNewUserAndSendActivationEmail(
    email
  )

  res.json({
    message:
      'Registration successful. Check your email for a link to set a password.',
  })
}

export default {
  registerPage: expressify(registerPage),
  registrationStatus: expressify(registrationStatus),
  registerWithUsernameAndPassword: expressify(registerWithUsernameAndPassword),
  registerWithEmail: expressify(registerWithEmail),
}
