// Everything an operator used to set with an environment variable, and where
// it lands in Settings.
//
// The whole point of this table is that the admin page needs no knowledge of
// any particular setting: it renders whatever is here, and a setting is added
// by adding a row. The `env` column is what the value used to be read from,
// and is used once -- to seed the stored value on a deployment that already
// had it set, so that upgrading does not silently reset somebody's site.
//
// A setting that is not here stays in the environment. That is deliberate:
// the database connection cannot come from the database, and a secret that is
// generated once at install has no business being editable in a web page.

/**
 * @typedef {object} SettingDefinition
 * @property {string} key       stable id, used by the API and the UI
 * @property {string} path      dot path into the Settings object
 * @property {string} [env]     the variable this used to be read from
 * @property {string} section
 * @property {'string'|'text'|'boolean'|'number'|'password'|'select'|'json'} type
 * @property {string} label
 * @property {string} [help]
 * @property {any} [default]
 * @property {Array<{value: string, label: string}>} [options]
 * @property {boolean} [restart] true when the value is only read at startup
 */

export const SECTIONS = [
  { id: 'site', label: 'Site', help: 'What the site is called and where it lives.' },
  { id: 'registration', label: 'Sign up and sign in', help: 'Who may create an account, and how.' },
  { id: 'oauth', label: 'Sign in with Google and GitHub', help: 'Leave the client id blank to hide a provider.' },
  { id: 'email', label: 'Email', help: 'Used for invitations, password resets and notifications.' },
  { id: 'appearance', label: 'Appearance', help: 'Branding shown around the editor.' },
  { id: 'compile', label: 'Compiling', help: 'Limits applied to every LaTeX compile.' },
  { id: 'security', label: 'Security', help: 'Session and password rules.' },
]

/** @type {SettingDefinition[]} */
export const SETTINGS = [
  // -- site ----------------------------------------------------------------
  {
    key: 'siteUrl',
    path: 'siteUrl',
    env: 'OVERLEAF_SITE_URL',
    section: 'site',
    type: 'string',
    label: 'Site URL',
    help: 'The address people reach this site at. Links in emails and the OAuth callback are built from it, so it has to be right.',
    default: 'http://localhost',
  },
  {
    key: 'appName',
    path: 'appName',
    env: 'OVERLEAF_APP_NAME',
    section: 'site',
    type: 'string',
    label: 'Site name',
    default: 'Overleaf Community Edition',
  },
  {
    key: 'adminEmail',
    path: 'adminEmail',
    env: 'OVERLEAF_ADMIN_EMAIL',
    section: 'site',
    type: 'string',
    label: 'Administrator email',
    help: 'Shown to users as the address to contact for help.',
    default: '',
  },
  {
    key: 'siteLanguage',
    path: 'i18n.defaultLng',
    env: 'OVERLEAF_SITE_LANGUAGE',
    section: 'site',
    type: 'string',
    label: 'Default language',
    default: 'en',
  },
  {
    key: 'statusPageUrl',
    path: 'statusPageUrl',
    env: 'OVERLEAF_STATUS_PAGE_URL',
    section: 'site',
    type: 'string',
    label: 'Status page URL',
    default: '',
  },

  // -- registration --------------------------------------------------------
  {
    key: 'allowPublicRegistration',
    path: 'siteSettings.allowPublicRegistration',
    env: 'OVERLEAF_ALLOW_PUBLIC_REGISTRATION',
    section: 'registration',
    type: 'boolean',
    label: 'Anyone may create an account',
    help: 'When this is off, only an administrator can add users. The very first account is always allowed, so a new site can be set up.',
    default: true,
  },
  {
    key: 'registrationEmailDomains',
    path: 'siteSettings.registrationEmailDomains',
    env: 'OVERLEAF_ALLOW_PUBLIC_REGISTRATION_DOMAINS',
    section: 'registration',
    type: 'string',
    label: 'Restrict sign up to these email domains',
    help: 'Comma separated, for example example.com, *.example.org. Leave blank to allow any address.',
    default: '',
  },
  {
    key: 'allowAnonymousReadAndWriteSharing',
    path: 'allowAnonymousReadAndWriteSharing',
    env: 'OVERLEAF_ALLOW_ANONYMOUS_READ_AND_WRITE_SHARING',
    section: 'registration',
    type: 'boolean',
    label: 'Allow link sharing with people who have no account',
    default: false,
  },
  {
    key: 'restrictInvitesToExistingAccounts',
    path: 'restrictInvitesToExistingAccounts',
    env: 'OVERLEAF_RESTRICT_INVITES_TO_EXISTING_ACCOUNTS',
    section: 'registration',
    type: 'boolean',
    label: 'Only allow project invitations to existing accounts',
    default: false,
  },

  // -- oauth ---------------------------------------------------------------
  {
    key: 'googleClientId',
    path: 'siteSettings.oauth.google.clientId',
    env: 'OVERLEAF_GOOGLE_CLIENT_ID',
    section: 'oauth',
    type: 'string',
    label: 'Google client ID',
    help: 'From the Google Cloud console. The redirect URI to register there is <site URL>/auth/google/callback.',
    default: '',
  },
  {
    key: 'googleClientSecret',
    path: 'siteSettings.oauth.google.clientSecret',
    env: 'OVERLEAF_GOOGLE_CLIENT_SECRET',
    section: 'oauth',
    type: 'password',
    label: 'Google client secret',
    default: '',
  },
  {
    key: 'githubClientId',
    path: 'siteSettings.oauth.github.clientId',
    env: 'OVERLEAF_GITHUB_CLIENT_ID',
    section: 'oauth',
    type: 'string',
    label: 'GitHub client ID',
    help: 'From your GitHub OAuth app. The callback URL to register there is <site URL>/auth/github/callback.',
    default: '',
  },
  {
    key: 'githubClientSecret',
    path: 'siteSettings.oauth.github.clientSecret',
    env: 'OVERLEAF_GITHUB_CLIENT_SECRET',
    section: 'oauth',
    type: 'password',
    label: 'GitHub client secret',
    default: '',
  },
  {
    key: 'oauthCreatesAccounts',
    path: 'siteSettings.oauth.createsAccounts',
    section: 'oauth',
    type: 'boolean',
    label: 'Signing in with Google or GitHub may create an account',
    help: 'When this is off, someone can only sign in that way if an account with the same email already exists, or if they linked it from their account settings.',
    default: true,
  },

  // -- email ---------------------------------------------------------------
  {
    key: 'emailFromAddress',
    path: 'email.fromAddress',
    env: 'OVERLEAF_EMAIL_FROM_ADDRESS',
    section: 'email',
    type: 'string',
    label: 'From address',
    default: '',
  },
  {
    key: 'emailReplyTo',
    path: 'email.replyTo',
    env: 'OVERLEAF_EMAIL_REPLY_TO',
    section: 'email',
    type: 'string',
    label: 'Reply-to address',
    default: '',
  },
  {
    key: 'emailSmtpHost',
    path: 'email.parameters.host',
    env: 'OVERLEAF_EMAIL_SMTP_HOST',
    section: 'email',
    type: 'string',
    label: 'SMTP host',
    help: 'Leave blank to turn email off. Without it, password resets and invitations cannot be sent.',
    default: '',
  },
  {
    key: 'emailSmtpPort',
    path: 'email.parameters.port',
    env: 'OVERLEAF_EMAIL_SMTP_PORT',
    section: 'email',
    type: 'number',
    label: 'SMTP port',
    default: 587,
  },
  {
    key: 'emailSmtpSecure',
    path: 'email.parameters.secure',
    env: 'OVERLEAF_EMAIL_SMTP_SECURE',
    section: 'email',
    type: 'boolean',
    label: 'Connect with TLS',
    help: 'On for port 465. Off for 587, which upgrades the connection afterwards.',
    default: false,
  },
  {
    key: 'emailSmtpUser',
    path: 'email.parameters.auth.user',
    env: 'OVERLEAF_EMAIL_SMTP_USER',
    section: 'email',
    type: 'string',
    label: 'SMTP username',
    default: '',
  },
  {
    key: 'emailSmtpPass',
    path: 'email.parameters.auth.pass',
    env: 'OVERLEAF_EMAIL_SMTP_PASS',
    section: 'email',
    type: 'password',
    label: 'SMTP password',
    default: '',
  },
  {
    key: 'emailSmtpIgnoreTLS',
    path: 'email.parameters.ignoreTLS',
    env: 'OVERLEAF_EMAIL_SMTP_IGNORE_TLS',
    section: 'email',
    type: 'boolean',
    label: 'Do not upgrade the connection to TLS',
    default: false,
  },
  {
    key: 'emailCustomFooter',
    path: 'email.template.customFooter',
    env: 'OVERLEAF_CUSTOM_EMAIL_FOOTER',
    section: 'email',
    type: 'text',
    label: 'Footer added to every email',
    default: '',
  },

  // -- appearance ----------------------------------------------------------
  {
    key: 'navTitle',
    path: 'nav.title',
    env: 'OVERLEAF_NAV_TITLE',
    section: 'appearance',
    type: 'string',
    label: 'Title in the navigation bar',
    default: '',
  },
  {
    key: 'headerImageUrl',
    path: 'nav.custom_logo',
    env: 'OVERLEAF_HEADER_IMAGE_URL',
    section: 'appearance',
    type: 'string',
    label: 'Logo URL',
    default: '',
  },
  {
    key: 'leftFooter',
    path: 'nav.left_footer',
    env: 'OVERLEAF_LEFT_FOOTER',
    section: 'appearance',
    type: 'json',
    label: 'Left footer',
    help: 'A JSON array of {"text": "...", "url": "..."} entries.',
    default: '[]',
  },
  {
    key: 'rightFooter',
    path: 'nav.right_footer',
    env: 'OVERLEAF_RIGHT_FOOTER',
    section: 'appearance',
    type: 'json',
    label: 'Right footer',
    help: 'A JSON array of {"text": "...", "url": "..."} entries.',
    default: '[]',
  },
  {
    key: 'maintenanceMessage',
    path: 'maintenanceMessage',
    env: 'OVERLEAF_MAINTENANCE_MESSAGE',
    section: 'appearance',
    type: 'text',
    label: 'Banner shown on every page',
    help: 'Leave blank for no banner.',
    default: '',
  },

  // -- compile -------------------------------------------------------------
  {
    key: 'compileTimeout',
    path: 'defaultFeatures.compileTimeout',
    env: 'COMPILE_TIMEOUT',
    section: 'compile',
    type: 'number',
    label: 'Compile timeout, seconds',
    help: 'One of the features an account is given when it is created, so changing it applies to accounts made after the change. Existing accounts keep the timeout they were given.',
    default: 180,
  },
  {
    key: 'compileGroupDefault',
    path: 'defaultFeatures.compileGroup',
    section: 'compile',
    type: 'select',
    label: 'Compile group for new users',
    options: [
      { value: 'standard', label: 'standard' },
      { value: 'priority', label: 'priority' },
    ],
    default: 'standard',
  },

  // -- security ------------------------------------------------------------
  {
    key: 'secureCookie',
    path: 'secureCookie',
    env: 'OVERLEAF_SECURE_COOKIE',
    section: 'security',
    type: 'boolean',
    label: 'Only send the session cookie over HTTPS',
    help: 'Turn this on once the site is served over HTTPS. With it on and the site on plain HTTP, nobody can sign in.',
    default: false,
    restart: true,
  },
  {
    key: 'cookieSessionLength',
    path: 'cookieSessionLength',
    env: 'OVERLEAF_COOKIE_SESSION_LENGTH',
    section: 'security',
    type: 'number',
    label: 'How long a session lasts, seconds',
    default: 5 * 24 * 60 * 60,
    restart: true,
  },
  {
    key: 'passwordMinLength',
    path: 'passwordStrengthOptions.length.min',
    env: 'OVERLEAF_PASSWORD_VALIDATION_MIN_LENGTH',
    section: 'security',
    type: 'number',
    label: 'Minimum password length',
    default: 8,
  },
  {
    key: 'passwordMaxLength',
    path: 'passwordStrengthOptions.length.max',
    env: 'OVERLEAF_PASSWORD_VALIDATION_MAX_LENGTH',
    section: 'security',
    type: 'number',
    label: 'Maximum password length',
    default: 72,
  },
]

const BY_KEY = new Map(SETTINGS.map(setting => [setting.key, setting]))

export function definitionFor(key) {
  return BY_KEY.get(key)
}

/** A value that is never sent to the browser in full. */
export function isSecret(definition) {
  return definition.type === 'password'
}
