import Settings from '@overleaf/settings'
import logger from '@overleaf/logger'
import { db } from '../../../../app/src/infrastructure/mongodb.mjs'
import { User } from '../../../../app/src/models/User.mjs'
import EmailHelper from '../../../../app/src/Features/Helpers/EmailHelper.mjs'

// Who may create an account, decided when somebody asks rather than when the
// process started.
//
// It has to be decided at request time because the answer now comes from the
// admin page: a site whose administrator turns registration on should not need
// a restart before anybody can use it. That is why the routes are registered
// unconditionally and refuse here instead of not existing.

/**
 * Whether the site has an administrator yet.
 *
 * A site with none is a site nobody has set up, and the first person to
 * register gets it -- which is how a fresh deployment is claimed without a
 * token in a log file or a shell inside the container.
 */
async function noAdminExists() {
  const admin = await db.users.findOne({ isAdmin: true }, { projection: { _id: 1 } })
  return admin == null
}

/** Splits the configured domain list, which may be empty. */
function allowedDomains() {
  const raw = Settings.siteSettings?.registrationEmailDomains || ''
  return raw
    .split(',')
    .map(entry => entry.trim().replace(/^@/, ''))
    .filter(Boolean)
}

/** Whether an address is in one of the allowed domains. `*.` matches below. */
function domainAllowed(email) {
  const patterns = allowedDomains()
  if (patterns.length === 0) {
    return true
  }
  const domain = EmailHelper.getDomain(email)
  if (!domain) {
    return false
  }
  return patterns.some(pattern => {
    if (pattern.startsWith('*.')) {
      const base = pattern.slice(2).toLowerCase()
      return domain === base || domain.endsWith(`.${base}`)
    }
    return domain === pattern.toLowerCase()
  })
}

const RegistrationPolicy = {
  noAdminExists,
  domainAllowed,
  allowedDomains,

  /**
   * What the sign-up page should offer, which is also what the API reports so
   * that something other than this page can ask.
   */
  async describe() {
    const claiming = await noAdminExists()
    return {
      // Somebody has to be able to claim a new site, so the first account is
      // always allowed however registration is configured.
      canRegister: claiming || Settings.siteSettings?.allowPublicRegistration === true,
      isFirstUser: claiming,
      allowedDomains: allowedDomains(),
      providers: Settings.siteSettings?.enabledOAuthProviders || [],
    }
  },

  /**
   * Whether this address may register, and why not when it may not.
   *
   * @returns {Promise<{allowed: boolean, isFirstUser: boolean, reason?: string}>}
   */
  async mayRegister(email) {
    const isFirstUser = await noAdminExists()
    if (isFirstUser) {
      return { allowed: true, isFirstUser: true }
    }
    if (Settings.siteSettings?.allowPublicRegistration !== true) {
      return {
        allowed: false,
        isFirstUser: false,
        reason: 'Registration is closed on this site.',
      }
    }
    if (!domainAllowed(email)) {
      return {
        allowed: false,
        isFirstUser: false,
        reason: `Sign up is limited to these email domains: ${allowedDomains().join(', ')}`,
      }
    }
    return { allowed: true, isFirstUser: false }
  },

  /**
   * Makes the first account an administrator.
   *
   * The check is repeated here under a condition that only matches while there
   * is still no admin, so two people registering at the same moment cannot
   * both be promoted: the second update matches nothing.
   */
  async promoteIfFirstUser(userId) {
    const existing = await db.users.findOne(
      { isAdmin: true },
      { projection: { _id: 1 } }
    )
    if (existing) {
      return false
    }
    const result = await User.updateOne(
      { _id: userId, isAdmin: { $ne: true } },
      { $set: { isAdmin: true } }
    ).exec()
    if (result.modifiedCount === 1) {
      logger.info({ userId }, 'first account on this site, made an administrator')
      return true
    }
    return false
  },
}

export default RegistrationPolicy
