/**
 * The modules wired into each injection point.
 *
 * This replaces `macros/import-overleaf-module.macro`, which read the same
 * table out of `settings.defaults.js` and expanded it into static imports at
 * build time. A Babel macro cannot run here, so the expansion it produced is
 * written out instead -- same imports, same shape, same eagerness.
 *
 * Generated from config/settings.defaults.js `overleafModuleImports`.
 */
import * as m1 from '@modules/reference-picker/js/extensions/reference-picker-keybinding'
import * as m2 from '@modules/reference-picker/js/extensions/reference-search-hint'
import * as m3 from '@modules/tpr-webmodule/js/components/create-file-mode-zotero'
import * as m4 from '@modules/tpr-webmodule/js/components/create-file-mode-mendeley'
import * as m5 from '@modules/template-gallery/js/components/actions-manage-template'
import * as m6 from '@modules/git-bridge/js/components/git-bridge-modal'
import * as m7 from '@modules/full-project-search/js/components/full-project-search'
import * as m8 from '@modules/full-project-search/js/components/full-project-search-button'
import * as m9 from '@modules/full-project-search/js/components/full-project-search'
import * as m10 from '@modules/github-sync/js/components/import-from-github-menu'
import * as m11 from '@modules/github-sync/js/components/import-from-github-modal-wrapper'
import * as m12 from '@modules/github-sync/js/components/github-sync-widget'
import * as m13 from '@modules/git-bridge/js/components/git-bridge-integration-card'
import * as m14 from '@modules/github-sync/js/components/github-integration-card'
import * as m15 from '@modules/tpr-webmodule/js/components/zotero-integration-card'
import * as m16 from '@modules/tpr-webmodule/js/components/mendeley-integration-card'
import * as m17 from '@modules/reference-picker/js/components/reference-picker-controller'
import * as m18 from '@modules/template-gallery/js/components/menubar-manage-template'
import * as m19 from '@modules/oauth2-server/js/components/git-integration'
import * as m20 from '@modules/python-runner/js/components/layout/python-editor-split'
import * as m21 from '@modules/reference-picker/js/reference-index/advanced-reference-index'
import * as m22 from '@modules/tpr-webmodule/js/components/zotero-widget'
import * as m23 from '@modules/tpr-webmodule/js/components/mendeley-widget'
import * as m24 from '@modules/symbol-palette/js/components/symbol-palette'
import * as m25 from '@modules/python-runner/js/components/editor/python/python-output-toasts'
import * as m26 from '@modules/tpr-webmodule/js/components/tpr-file-view-info'
import * as m27 from '@modules/tpr-webmodule/js/components/tpr-file-view-not-original-importer'
import * as m28 from '@modules/tpr-webmodule/js/components/tpr-file-view-refresh-button'
import * as m29 from '@modules/tpr-webmodule/js/components/tpr-file-view-refresh-error'

export type OverleafModule = { import: any; path: string }

const modules: Record<string, OverleafModule[]> = {
  autoCompleteExtensions: [
    { import: m1, path: '@modules/reference-picker/js/extensions/reference-picker-keybinding' },
    { import: m2, path: '@modules/reference-picker/js/extensions/reference-search-hint' },
  ],
  contactUsModal: [],
  createFileModes: [
    { import: m3, path: '@modules/tpr-webmodule/js/components/create-file-mode-zotero' },
    { import: m4, path: '@modules/tpr-webmodule/js/components/create-file-mode-mendeley' },
  ],
  devToolbar: [],
  diagnosticActions: [],
  editorLeftMenuManageTemplate: [
    { import: m5, path: '@modules/template-gallery/js/components/actions-manage-template' },
  ],
  editorLeftMenuSync: [
    { import: m6, path: '@modules/git-bridge/js/components/git-bridge-modal' },
  ],
  editorSidebarComponents: [
    { import: m7, path: '@modules/full-project-search/js/components/full-project-search' },
  ],
  errorLogsComponents: [],
  fileTreeToolbarComponents: [
    { import: m8, path: '@modules/full-project-search/js/components/full-project-search-button' },
  ],
  fullProjectSearchPanel: [
    { import: m9, path: '@modules/full-project-search/js/components/full-project-search' },
  ],
  gitBridge: [],
  importProjectFromGithubMenu: [
    { import: m10, path: '@modules/github-sync/js/components/import-from-github-menu' },
  ],
  importProjectFromGithubModalWrapper: [
    { import: m11, path: '@modules/github-sync/js/components/import-from-github-modal-wrapper' },
  ],
  insertMenuSections: [],
  integrationLinkingWidgets: [
    { import: m12, path: '@modules/github-sync/js/components/github-sync-widget' },
  ],
  integrationPanelComponents: [
    { import: m13, path: '@modules/git-bridge/js/components/git-bridge-integration-card' },
    { import: m14, path: '@modules/github-sync/js/components/github-integration-card' },
    { import: m15, path: '@modules/tpr-webmodule/js/components/zotero-integration-card' },
    { import: m16, path: '@modules/tpr-webmodule/js/components/mendeley-integration-card' },
  ],
  labsExperiments: [],
  langFeedbackLinkingWidgets: [],
  mainEditorLayoutModals: [
    { import: m17, path: '@modules/reference-picker/js/components/reference-picker-controller' },
  ],
  mainEditorLayoutPanels: [],
  managedGroupEnrollmentInvite: [],
  managedGroupSubscriptionEnrollmentNotification: [],
  menubarExtraComponents: [
    { import: m18, path: '@modules/template-gallery/js/components/menubar-manage-template' },
  ],
  oauth2Server: [
    { import: m19, path: '@modules/oauth2-server/js/components/git-integration' },
  ],
  offlineModeToolbarButtons: [],
  pdfLogEntriesComponents: [],
  pdfLogEntryComponents: [],
  pdfLogEntryHeaderActionComponents: [],
  pdfPreviewPromotions: [],
  publishModal: [],
  pythonRunner: [
    { import: m20, path: '@modules/python-runner/js/components/layout/python-editor-split' },
  ],
  railActions: [],
  railEntries: [],
  railModals: [],
  railPopovers: [],
  referenceIndices: [
    { import: m21, path: '@modules/reference-picker/js/reference-index/advanced-reference-index' },
  ],
  referenceLinkingWidgets: [
    { import: m22, path: '@modules/tpr-webmodule/js/components/zotero-widget' },
    { import: m23, path: '@modules/tpr-webmodule/js/components/mendeley-widget' },
  ],
  referenceSearchSetting: [],
  rollingBuildsUpdatedAlert: [],
  rootContextProviders: [],
  sectionTitleGenerators: [],
  settingsEntries: [],
  settingsModalEditorTabSections: [],
  settingsModalSpellcheckSections: [],
  snapshotUtils: [],
  sourceEditorCompletionSources: [],
  sourceEditorComponents: [],
  sourceEditorExtensions: [],
  sourceEditorSymbolPalette: [
    { import: m24, path: '@modules/symbol-palette/js/components/symbol-palette' },
  ],
  sourceEditorToolbarButtonGroups: [],
  sourceEditorToolbarComponents: [],
  sourceEditorToolbarEndButtons: [],
  sourceEditorVisualExtensions: [],
  ssoCertificateInfo: [],
  toastGenerators: [
    { import: m25, path: '@modules/python-runner/js/components/editor/python/python-output-toasts' },
  ],
  tprFileViewInfo: [
    { import: m26, path: '@modules/tpr-webmodule/js/components/tpr-file-view-info' },
  ],
  tprFileViewNotOriginalImporter: [
    { import: m27, path: '@modules/tpr-webmodule/js/components/tpr-file-view-not-original-importer' },
  ],
  tprFileViewRefreshButton: [
    { import: m28, path: '@modules/tpr-webmodule/js/components/tpr-file-view-refresh-button' },
  ],
  tprFileViewRefreshError: [
    { import: m29, path: '@modules/tpr-webmodule/js/components/tpr-file-view-refresh-error' },
  ],
  usGovBanner: [],
  v1ImportDataScreen: [],
  visualEditorProviders: [],
}

export default function importOverleafModules(name: string): OverleafModule[] {
  const found = modules[name]
  if (!found) {
    // The macro threw at build time for an unknown key. Nothing here can throw
    // at build time, so an unknown key is an empty injection point instead --
    // which is what every unused key already is.
    return []
  }
  return found
}
