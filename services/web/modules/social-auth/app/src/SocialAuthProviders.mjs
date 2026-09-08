import Settings from '@overleaf/settings'

// Signing in with Google or GitHub.
//
// Both are ordinary OAuth2 authorization-code flows and differ only in where
// the endpoints are and what the profile looks like, so they are described
// here as data and the rest of the module treats them alike. Adding a third
// provider is adding an entry.
//
// The client id and secret come from the settings store rather than the
// environment, so a provider can be turned on from the admin page without a
// restart -- which is why nothing here is read at import time.

/**
 * @typedef {object} ProviderDefinition
 * @property {string} id
 * @property {string} name
 * @property {(options: object) => import('passport-strategy')} buildStrategy
 * @property {(profile: object) => {externalUserId: string, email?: string,
 *   firstName?: string, lastName?: string}} normalise
 */

/** The scopes each provider needs to give us an email address. */
const SCOPES = {
  google: ['profile', 'email'],
  github: ['read:user', 'user:email'],
}

/**
 * GitHub does not put a verified email in the profile when the user has made
 * it private, so it is fetched separately and the primary verified one taken.
 */
async function fetchGithubEmail(accessToken) {
  const response = await fetch('https://api.github.com/user/emails', {
    headers: {
      Authorization: `Bearer ${accessToken}`,
      Accept: 'application/vnd.github+json',
      'User-Agent': 'overleaf',
    },
  })
  if (!response.ok) {
    return undefined
  }
  const emails = await response.json()
  if (!Array.isArray(emails)) {
    return undefined
  }
  const primary = emails.find(entry => entry.primary && entry.verified)
  const verified = emails.find(entry => entry.verified)
  return (primary || verified)?.email
}

/** The GitHub profile, fetched with the token we were just given. */
async function fetchGithubProfile(accessToken) {
  const response = await fetch('https://api.github.com/user', {
    headers: {
      Authorization: `Bearer ${accessToken}`,
      Accept: 'application/vnd.github+json',
      'User-Agent': 'overleaf',
    },
  })
  if (!response.ok) {
    throw new Error(`GitHub refused the profile request: ${response.status}`)
  }
  const profile = await response.json()
  if (!profile.email) {
    profile.email = await fetchGithubEmail(accessToken)
  }
  return profile
}

export const PROVIDERS = {
  google: {
    id: 'google',
    name: 'Google',
    scopes: SCOPES.google,
    async buildStrategy({ clientId, clientSecret, callbackURL, verify }) {
      const { Strategy } = await import('passport-google-oauth20')
      return new Strategy(
        {
          clientID: clientId,
          clientSecret,
          callbackURL,
          scope: SCOPES.google,
          passReqToCallback: true,
        },
        verify
      )
    },
    normalise(profile) {
      const email = profile.emails?.[0]?.value
      return {
        externalUserId: profile.id,
        email,
        firstName: profile.name?.givenName || '',
        lastName: profile.name?.familyName || '',
      }
    },
  },

  github: {
    id: 'github',
    name: 'GitHub',
    scopes: SCOPES.github,
    async buildStrategy({ clientId, clientSecret, callbackURL, verify }) {
      const { Strategy } = await import('passport-oauth2')
      const strategy = new Strategy(
        {
          authorizationURL: 'https://github.com/login/oauth/authorize',
          tokenURL: 'https://github.com/login/oauth/access_token',
          clientID: clientId,
          clientSecret,
          callbackURL,
          scope: SCOPES.github,
          passReqToCallback: true,
        },
        verify
      )
      // passport-oauth2 has no idea what a GitHub profile is, so it is fetched
      // here and handed back in the shape the verify callback expects.
      strategy.userProfile = function (accessToken, done) {
        fetchGithubProfile(accessToken).then(
          profile => done(null, profile),
          error => done(error)
        )
      }
      return strategy
    },
    normalise(profile) {
      const [firstName, ...rest] = (profile.name || '').split(' ')
      return {
        externalUserId: String(profile.id),
        email: profile.email,
        firstName: firstName || profile.login || '',
        lastName: rest.join(' '),
      }
    },
  },
}

/** The settings for one provider, or null when it has not been configured. */
export function configFor(providerId) {
  const definition = PROVIDERS[providerId]
  if (!definition) {
    return null
  }
  const configured = Settings.siteSettings?.oauth?.[providerId] || {}
  if (!configured.clientId || !configured.clientSecret) {
    return null
  }
  return {
    clientId: configured.clientId,
    clientSecret: configured.clientSecret,
    callbackURL: `${(Settings.siteUrl || '').replace(/\/+$/, '')}/auth/${providerId}/callback`,
  }
}

/** The providers an administrator has configured, for the sign-in page. */
export function enabledProviders() {
  return Object.values(PROVIDERS)
    .filter(definition => configFor(definition.id) != null)
    .map(definition => ({
      id: definition.id,
      name: definition.name,
      linkPath: `/auth/${definition.id}`,
    }))
}
