import router from './app/src/SiteSettingsRouter.mjs'
import SiteSettingsManager from './app/src/SiteSettingsManager.mjs'

// The settings an operator used to put in the environment are read from the
// database instead, and changed from a page rather than by editing a file and
// restarting. See app/src/SiteSettingsCatalogue.mjs for what moved and what
// deliberately did not.

export default {
  name: 'site-settings',
  router,
  SiteSettingsManager,
}
