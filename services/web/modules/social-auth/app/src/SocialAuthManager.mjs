import crypto from 'node:crypto'
import Settings from '@overleaf/settings'
import logger from '@overleaf/logger'
import UserCreator from '../../../../app/src/Features/User/UserCreator.mjs'
import UserGetter from '../../../../app/src/Features/User/UserGetter.mjs'
import ThirdPartyIdentityManager from '../../../../app/src/Features/User/ThirdPartyIdentityManager.mjs'
import { User } from '../../../../app/src/models/User.mjs'
import RegistrationPolicy from '../../../login-register/app/src/RegistrationPolicy.mjs'

// Turning a Google or GitHub sign-in into an account here.
//
// One person may hold several of these -- a Google account, a GitHub account
// and a password -- and they are meant to be one account, not three. What
// makes them one is the email address: an identity whose verified email
// matches an existing account is attached to that account rather than starting
// a new one. Anything else would silently give somebody a second, empty
// account the first time they used a different button.
//
// The identities themselves are held by ThirdPartyIdentityManager, which is
// what core already uses for this, so an account linked here looks exactly
// like one linked any other way.

/** An identity that is already attached to somebody. */
async function findByIdentity(providerId, externalUserId) {
  try {
    return await ThirdPartyIdentityManager.promises.login(
      providerId,
      externalUserId,
      null
    )
  } catch {
    return null
  }
}

const SocialAuthManager = {
  /**
   * Finds the account this identity belongs to, attaching it to an existing
   * one or making a new one.
   *
   * @returns {Promise<{user: object|null, reason?: string}>}
   */
  async findOrCreateUser(providerId, identity, auditLog) {
    const { externalUserId, email, firstName, lastName } = identity

    if (!externalUserId) {
      return { user: null, reason: `${providerId} did not identify the account` }
    }

    // Already linked: this is simply a sign-in.
    const linked = await findByIdentity(providerId, externalUserId)
    if (linked) {
      return { user: linked }
    }

    if (!email) {
      // Without an address there is nothing to match on and nothing to create
      // an account with. GitHub does this when every address is private and
      // unverified.
      return {
        user: null,
        reason:
          `${providerId} did not give us a verified email address. Add one to that account, or sign in another way and link it from your account settings.`,
      }
    }

    // The same person, arriving by a door they have not used before.
    const existing = await UserGetter.promises.getUserByAnyEmail(email)
    if (existing) {
      auditLog.initiatorId = existing._id
      await ThirdPartyIdentityManager.promises.link(
        existing._id,
        providerId,
        externalUserId,
        null,
        auditLog
      )
      logger.info(
        { userId: existing._id, providerId },
        'linked a social identity to the account with the same email'
      )
      return { user: existing }
    }

    // Nobody holds that address, so this would be a new account.
    if (Settings.siteSettings?.oauth?.createsAccounts !== true) {
      return {
        user: null,
        reason:
          'This site does not create accounts from Google or GitHub. Ask an administrator for an account, then link it from your account settings.',
      }
    }
    const decision = await RegistrationPolicy.mayRegister(email)
    if (!decision.allowed) {
      return { user: null, reason: decision.reason }
    }

    const user = await UserCreator.promises.createNewUser({
      email,
      first_name: firstName || '',
      last_name: lastName || '',
      holdingAccount: false,
      analyticsId: crypto.randomUUID(),
    })

    auditLog.initiatorId = user._id
    await ThirdPartyIdentityManager.promises.link(
      user._id,
      providerId,
      externalUserId,
      null,
      auditLog
    )

    // The provider vouched for the address, so it does not need confirming
    // again by email -- which matters on a site with no mail server.
    await User.updateOne(
      { _id: user._id },
      { $set: { 'emails.0.confirmedAt': new Date() } }
    ).exec()

    if (decision.isFirstUser) {
      await RegistrationPolicy.promoteIfFirstUser(user._id)
    }

    return { user: await UserGetter.promises.getUser(user._id) }
  },

  /**
   * Attaches this identity to an account that is already signed in.
   *
   * This is the other direction: somebody with a password who wants the button
   * to work next time. It refuses an identity that already belongs to a
   * different account rather than moving it, because moving it would quietly
   * lock the other account out of that provider.
   */
  async linkToUser(userId, providerId, identity, auditLog) {
    const { externalUserId } = identity
    if (!externalUserId) {
      return { linked: false, reason: `${providerId} did not identify the account` }
    }

    const holder = await findByIdentity(providerId, externalUserId)
    if (holder && String(holder._id) !== String(userId)) {
      return {
        linked: false,
        reason: `That ${providerId} account is already linked to another account here.`,
      }
    }
    if (holder) {
      return { linked: true }
    }

    auditLog.initiatorId = userId
    await ThirdPartyIdentityManager.promises.link(
      userId,
      providerId,
      externalUserId,
      null,
      auditLog
    )
    return { linked: true }
  },

  /**
   * Detaches an identity.
   *
   * Somebody whose only way in is the identity being removed would be locked
   * out, so that is refused: they have to set a password, or keep another
   * provider linked.
   */
  async unlinkFromUser(userId, providerId, auditLog) {
    const user = await UserGetter.promises.getUser(userId, {
      hashedPassword: 1,
      thirdPartyIdentifiers: 1,
    })
    if (!user) {
      return { unlinked: false, reason: 'no such account' }
    }

    const identities = user.thirdPartyIdentifiers || []
    const remaining = identities.filter(
      identity => identity.providerId !== providerId
    )
    if (remaining.length === identities.length) {
      return { unlinked: false, reason: 'that provider is not linked' }
    }
    if (remaining.length === 0 && !user.hashedPassword) {
      return {
        unlinked: false,
        reason:
          'That is the only way you can sign in. Set a password first, or link another provider.',
      }
    }

    // The audit entry is about this account acting on itself, the same way
    // linking is. Leaving it to every caller to remember is how the audit log
    // ends up refusing the entry.
    auditLog.initiatorId = auditLog.initiatorId || userId
    await ThirdPartyIdentityManager.promises.unlink(userId, providerId, auditLog)
    return { unlinked: true }
  },
}

export default SocialAuthManager
