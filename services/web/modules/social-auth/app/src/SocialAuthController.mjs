import passport from 'passport'
import logger from '@overleaf/logger'
import Settings from '@overleaf/settings'
import { expressify } from '@overleaf/promise-utils'
import AuthenticationController from '../../../../app/src/Features/Authentication/AuthenticationController.mjs'
import SessionManager from '../../../../app/src/Features/Authentication/SessionManager.mjs'
import SocialAuthManager from './SocialAuthManager.mjs'
import { PROVIDERS, configFor, enabledProviders } from './SocialAuthProviders.mjs'

// The strategies are built when they are used rather than at startup, because
// an administrator can add a client id from the admin page and should not have
// to restart the site before the button works. passport keys strategies by
// name, so registering one again simply replaces it.

/** The configuration a strategy was last built with, so it is rebuilt on change. */
const builtWith = new Map()

async function ensureStrategy(providerId) {
  const definition = PROVIDERS[providerId]
  const config = configFor(providerId)
  if (!definition || !config) {
    return false
  }

  const fingerprint = `${config.clientId}:${config.clientSecret}:${config.callbackURL}`
  if (builtWith.get(providerId) === fingerprint) {
    return true
  }

  const verify = (req, accessToken, refreshToken, profile, done) => {
    // The profile is normalised here so that everything below this point is
    // the same for every provider.
    let identity
    try {
      identity = definition.normalise(profile)
    } catch (error) {
      return done(error)
    }
    req.socialIdentity = identity
    done(null, { identity })
  }

  const strategy = await definition.buildStrategy({ ...config, verify })
  passport.use(`social-${providerId}`, strategy)
  builtWith.set(providerId, fingerprint)
  return true
}

/** Starts the flow. `?intent=link` attaches it to the account already signed in. */
async function start(req, res, next) {
  const providerId = req.params.provider
  if (!(await ensureStrategy(providerId))) {
    return res.status(404).render('general/404', { title: 'Not found' })
  }

  if (req.query.intent === 'link') {
    if (!SessionManager.isUserLoggedIn(req.session)) {
      return res.redirect('/login')
    }
    req.session.socialAuthIntent = 'link'
  } else {
    delete req.session.socialAuthIntent
  }

  passport.authenticate(`social-${providerId}`, {
    scope: PROVIDERS[providerId].scopes,
    session: false,
  })(req, res, next)
}

/** Where the provider sends the person back to. */
async function callback(req, res, next) {
  const providerId = req.params.provider
  if (!(await ensureStrategy(providerId))) {
    return res.status(404).render('general/404', { title: 'Not found' })
  }

  const intent = req.session.socialAuthIntent
  delete req.session.socialAuthIntent

  passport.authenticate(
    `social-${providerId}`,
    { session: false, keepSessionInfo: true },
    async (err, result) => {
      if (err) {
        logger.warn({ err, providerId }, 'social sign-in failed')
        return _fail(req, res, intent, 'That sign-in did not complete.')
      }
      if (!result?.identity) {
        return _fail(req, res, intent, 'That sign-in did not complete.')
      }

      const auditLog = {
        ipAddress: req.ip,
        info: { method: `${providerId} login` },
      }

      try {
        if (intent === 'link') {
          const userId = SessionManager.getLoggedInUserId(req.session)
          if (!userId) {
            return res.redirect('/login')
          }
          const { linked, reason } = await SocialAuthManager.linkToUser(
            userId,
            providerId,
            result.identity,
            auditLog
          )
          return _finishLink(req, res, linked, reason)
        }

        const { user, reason } = await SocialAuthManager.findOrCreateUser(
          providerId,
          result.identity,
          auditLog
        )
        if (!user) {
          return _fail(req, res, intent, reason)
        }

        AuthenticationController.setAuditInfo(req, {
          method: `${providerId} login`,
        })
        await AuthenticationController.promises.finishLogin(user, req, res)
      } catch (error) {
        return next(error)
      }
    }
  )(req, res, next)
}

function _fail(req, res, intent, reason) {
  const message = reason || 'That sign-in did not complete.'
  if (intent === 'link') {
    return _finishLink(req, res, false, message)
  }
  req.session.socialAuthError = message
  return res.redirect('/login')
}

function _finishLink(req, res, linked, reason) {
  if (!linked) {
    req.session.socialAuthError = reason
  }
  return res.redirect('/user/settings')
}

/** Detaches a provider from the account that is signed in. */
async function unlink(req, res) {
  const providerId = req.params.provider || req.body?.providerId
  const userId = SessionManager.getLoggedInUserId(req.session)
  if (!PROVIDERS[providerId]) {
    return res.status(400).json({ message: 'no such provider' })
  }

  const { unlinked, reason } = await SocialAuthManager.unlinkFromUser(
    userId,
    providerId,
    { ipAddress: req.ip, initiatorId: userId, info: { method: 'unlink' } }
  )
  if (!unlinked) {
    return res.status(400).json({ message: reason })
  }
  res.json({ message: `${PROVIDERS[providerId].name} unlinked` })
}

/** Which providers are configured, and which the signed-in account has linked. */
async function status(req, res) {
  const user = SessionManager.getSessionUser(req.session)
  let linked = []
  if (user) {
    const { default: UserGetter } = await import(
      '../../../../app/src/Features/User/UserGetter.mjs'
    )
    const full = await UserGetter.promises.getUser(user._id, {
      thirdPartyIdentifiers: 1,
    })
    linked = (full?.thirdPartyIdentifiers || []).map(
      identity => identity.providerId
    )
  }
  res.json({ providers: enabledProviders(), linked })
}

export default {
  start: expressify(start),
  callback: expressify(callback),
  unlink: expressify(unlink),
  status: expressify(status),
}
