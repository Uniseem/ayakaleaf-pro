import Path from 'node:path'
import logger from '@overleaf/logger'
import Settings from '@overleaf/settings'
import { expressify } from '@overleaf/promise-utils'
import SiteSettingsManager from './SiteSettingsManager.mjs'
import SessionManager from '../../../../app/src/Features/Authentication/SessionManager.mjs'

// The page is a thin consumer of the API rather than the other way round: the
// same JSON that draws it is what a script, or one day an app, would use.

async function settingsPage(req, res) {
  res.render(Path.resolve(import.meta.dirname, '../views/settings'), {
    title: 'Settings',
  })
}

async function getSettings(req, res) {
  res.json(SiteSettingsManager.describe())
}

async function updateSettings(req, res) {
  const changes = req.body?.settings
  if (changes == null || typeof changes !== 'object' || Array.isArray(changes)) {
    return res.status(400).json({ message: 'expected a settings object' })
  }

  const adminUserId = SessionManager.getLoggedInUserId(req.session)
  const applied = await SiteSettingsManager.save(changes, adminUserId)

  res.json({ applied, settings: SiteSettingsManager.describe() })
}

/**
 * Sends an email to the signed-in administrator, so that a mail server can be
 * checked from the page that configured it rather than by creating an account
 * and asking for a password reset.
 */
async function sendTestEmail(req, res) {
  const { default: EmailHandler } = await import(
    '../../../../app/src/Features/Email/EmailHandler.mjs'
  )
  const user = SessionManager.getSessionUser(req.session)
  const to = req.body?.email || user?.email
  if (!to) {
    return res.status(400).json({ message: 'no address to send to' })
  }
  if (!Settings.email) {
    return res
      .status(400)
      .json({ message: 'email is not configured: set an SMTP host first' })
  }

  try {
    await EmailHandler.promises.sendEmail('testEmail', { to })
    res.json({ message: `sent to ${to}` })
  } catch (err) {
    logger.warn({ err }, 'test email failed')
    res.status(500).json({ message: err.message })
  }
}

export default {
  settingsPage: expressify(settingsPage),
  getSettings: expressify(getSettings),
  updateSettings: expressify(updateSettings),
  sendTestEmail: expressify(sendTestEmail),
}
