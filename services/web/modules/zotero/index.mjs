import Settings from '@overleaf/settings'
import ZoteroRouter from './app/src/ZoteroRouter.mjs'

let ZoteroModule = {}

if (Settings.enabledLinkedFileTypes?.includes('zotero')) {
  const siteUrl =
    process.env.OVERLEAF_SITE_URL?.replace(/\/+$/, '') || Settings.siteUrl
  Settings.zotero = {
    clientID: process.env.ZOTERO_CLIENT_ID,
    clientSecret: process.env.ZOTERO_CLIENT_SECRET,
    callbackURL: `${siteUrl}/user/zotero/oauth/callback`,
  }

  const { default: ZoteroLinkedFileAgent } = await import(
    './app/src/ZoteroLinkedFileAgent.mjs'
  )

  ZoteroModule = {
    router: ZoteroRouter,
    linkedFileAgents: {
      zotero: () => ZoteroLinkedFileAgent,
    },
  }
}

export default ZoteroModule
