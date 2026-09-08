import Settings from '@overleaf/settings'
import logger from '@overleaf/logger'
import { db } from '../../../../app/src/infrastructure/mongodb.mjs'
import { User } from '../../../../app/src/models/User.mjs'
import EmailHelper from '../../../../app/src/Features/Helpers/EmailHelper.mjs'

// The document whose existence means the first-administrator place is taken.
// It lives beside the settings because it is the same kind of thing: one fact
// about this site, held once.
const ADMIN_CLAIM_ID = 'first-admin-claim'

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
   * Two people registering in the same second must not both get it, and no
   * query over the users collection can settle that -- both would read no
   * administrator and both would then write one. So the claim is an insert of
   * a single document with a fixed id: Mongo lets exactly one of them succeed
   * and gives the other a duplicate key, which is the whole of the tie-break.
   *
   * The check for an administrator who already exists is still made first, and
   * separately: it is what stops the claim being available on a site that has
   * had an administrator all along.
   */
  async promoteIfFirstUser(userId) {
    if (!(await noAdminExists())) {
      return false
    }

    try {
      await db.siteSettings.insertOne({
        _id: ADMIN_CLAIM_ID,
        userId: String(userId),
        claimedAt: new Date(),
      })
    } catch (error) {
      if (error.code === 11000) {
        // Somebody else was a moment quicker.
        return false
      }
      throw error
    }

    try {
      await User.updateOne(
        { _id: userId, isAdmin: { $ne: true } },
        { $set: { isAdmin: true } }
      ).exec()
    } catch (error) {
      // Holding a claim nobody can act on would leave the site with no way to
      // get an administrator at all, so it is given back.
      await db.siteSettings.deleteOne({ _id: ADMIN_CLAIM_ID })
      throw error
    }

    logger.info({ userId }, 'first account on this site, made an administrator')
    return true
  },
}

export default RegistrationPolicy
