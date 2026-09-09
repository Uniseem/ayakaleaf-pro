'use client'

/**
 * The integrations panel in the rail, from
 * integrations-panel/integrations-panel.tsx: a header and a card for each
 * thing the project can be connected to. On this deployment those are git
 * and GitHub.
 */

import { useEffect, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { RailPanelHeader } from '@/features/ide/components/rail/rail-parts'
import { useProject } from '@/features/ide/contexts/project-context'
import { projectGitHub, type ProjectGitHub } from '@/lib/github'
import { GitHubPanel } from '@/app/projects/[id]/github-panel'

export function IntegrationsPanel() {
  const { t } = useTranslation()
  const { projectId, project } = useProject()
  const [status, setStatus] = useState<ProjectGitHub | null>(null)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    projectGitHub(projectId)
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [projectId])

  return (
    <div className="integrations-panel">
      <RailPanelHeader title={t('integrations')} />
      <div className="integrations-panel-body">
        <a className="integrations-panel-card-button" href="/account" target="_blank" rel="noreferrer">
          <div className="integrations-panel-card-contents">
            <div className="integrations-panel-card-icon">
              <MaterialIcon type="code" unfilled />
            </div>
            <div className="integrations-panel-card-inner">
              <div className="integrations-panel-card-header">
                <div className="integrations-panel-card-title">Git</div>
              </div>
              <p className="integrations-panel-card-description">{t('git_integration_info')}</p>
            </div>
          </div>
        </a>
        <button type="button" className="integrations-panel-card-button" onClick={() => setOpen(true)}>
          <div className="integrations-panel-card-contents">
            <div className="integrations-panel-card-icon">
              <MaterialIcon type="cloud_sync" />
            </div>
            <div className="integrations-panel-card-inner">
              <div className="integrations-panel-card-header">
                <div className="integrations-panel-card-title">GitHub</div>
              </div>
              <p className="integrations-panel-card-description">{t('github_sync_description')}</p>
            </div>
          </div>
        </button>
      </div>
      {status ? (
        <GitHubPanel projectId={projectId} projectName={project.name} initial={status} open={open} onClose={() => setOpen(false)} />
      ) : null}
    </div>
  )
}
