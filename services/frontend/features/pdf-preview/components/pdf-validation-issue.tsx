'use client'

import { memo } from 'react'
import { useTranslation } from '@/lib/i18n'
import PdfLogEntry from './pdf-log-entry'
import type { ValidationProblems } from '../util/types'

type Issue = ValidationProblems[keyof ValidationProblems]

/** A problem the compiler found with the project before it started. */
function PdfValidationIssue({ issue, name }: { issue: Issue; name: string }) {
  const { t } = useTranslation()

  switch (name) {
    case 'sizeCheck': {
      const sizeCheck = issue as ValidationProblems['sizeCheck']
      return (
        <PdfLogEntry
          headerTitle={t('project_too_large')}
          formattedContent={
            <>
              <div>{t('project_too_large_please_reduce')}</div>
              <ul className="list-no-margin-bottom">
                {sizeCheck?.resources.map(resource => (
                  <li key={resource.path}>
                    {resource.path} &mdash; {resource.kbSize}
                    kb
                  </li>
                ))}
              </ul>
            </>
          }
          entryAriaLabel={t('validation_issue_entry_description')}
          level="error"
        />
      )
    }

    case 'conflictedPaths': {
      const conflictedPaths = issue as ValidationProblems['conflictedPaths']
      return (
        <PdfLogEntry
          headerTitle={t('conflicting_paths_found')}
          formattedContent={
            <>
              <div>{t('following_paths_conflict')}</div>
              <ul className="list-no-margin-bottom">
                {conflictedPaths?.map(detail => (
                  <li key={detail.path}>/{detail.path}</li>
                ))}
              </ul>
            </>
          }
          entryAriaLabel={t('validation_issue_entry_description')}
          level="error"
        />
      )
    }

    case 'mainFile':
      return (
        <PdfLogEntry
          headerTitle={t('main_file_not_found')}
          formattedContent={t('please_set_main_file')}
          entryAriaLabel={t('validation_issue_entry_description')}
          level="error"
        />
      )

    default:
      return null
  }
}

export default memo(PdfValidationIssue)
