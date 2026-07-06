import { useTranslation } from 'react-i18next'
import ZoteroLogo from '@/shared/svgs/zotero-logo'
import IntegrationCard from '@/features/integrations-panel/integration-card'

/**
 * Zotero card in the editor's Integrations panel. Clicking it takes the user to
 * the Reference managers section of Account Settings, where the Zotero account
 * can be linked or unlinked.
 *
 * Registered via overleafModuleImports.integrationPanelComponents.
 */
const ZoteroIntegrationCard = () => {
  const { t } = useTranslation()

  return (
    <IntegrationCard
      title={t('zotero')}
      description={t('cite_directly_or_import_references')}
      icon={<ZoteroLogo size={32} />}
      showPaywallBadge={false}
      onClick={() => window.location.assign('/user/settings#references')}
    />
  )
}

export default ZoteroIntegrationCard
