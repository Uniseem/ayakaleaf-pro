import logger from '@overleaf/logger'
import SiteSettingsController from './SiteSettingsController.mjs'
import AuthorizationMiddleware from '../../../../app/src/Features/Authorization/AuthorizationMiddleware.mjs'

export default {
  apply(webRouter) {
    logger.debug({}, 'Init site settings router')

    webRouter.get(
      '/admin/settings',
      AuthorizationMiddleware.ensureUserIsSiteAdmin,
      SiteSettingsController.settingsPage
    )
    webRouter.get(
      '/admin/settings/api',
      AuthorizationMiddleware.ensureUserIsSiteAdmin,
      SiteSettingsController.getSettings
    )
    webRouter.post(
      '/admin/settings/api',
      AuthorizationMiddleware.ensureUserIsSiteAdmin,
      SiteSettingsController.updateSettings
    )
    webRouter.post(
      '/admin/settings/test-email',
      AuthorizationMiddleware.ensureUserIsSiteAdmin,
      SiteSettingsController.sendTestEmail
    )
  },
}
