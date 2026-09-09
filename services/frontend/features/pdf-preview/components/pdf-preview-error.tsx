'use client'

/**
 * What to show for each way a compile can fail, from
 * pdf-preview/components/pdf-preview-error. Each is a log entry with a
 * title and an explanation; the timeout one also offers to stop on the
 * first error, which is usually the cure.
 */

import { memo, useCallback, type ReactNode } from 'react'
import { useTranslation, Trans } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import PdfLogEntry from './pdf-log-entry'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useStopOnFirstError } from '../hooks/use-stop-on-first-error'

const TROUBLESHOOTING_URL = '/learn/how-to/Resolving_access%2C_loading%2C_and_display_problems'

function PdfPreviewError({
  error,
  includeWarnings = true,
  includeErrors = true,
}: {
  error: string
  includeWarnings?: boolean
  includeErrors?: boolean
}) {
  const { t } = useTranslation()

  const { startCompile } = useCompile()

  switch (error) {
    case 'rendering-error-expected':
      return (
        includeWarnings && (
          <PdfLogEntry
            headerTitle={t('pdf_rendering_error')}
            formattedContent={
              <>
                <Trans
                  i18nKey="something_went_wrong_rendering_pdf_expected"
                  components={[<Button key="recompile" variant="primary" size="sm" onClick={() => startCompile()} />]}
                />
                <br />
                <br />
                <Trans
                  i18nKey="last_resort_trouble_shooting_guide"
                  components={[<a href={TROUBLESHOOTING_URL} target="_blank" key="troubleshooting-link" />]}
                />
              </>
            }
            level="warning"
          />
        )
      )

    case 'rendering-error':
      return (
        includeErrors && (
          <ErrorLogEntry title={t('pdf_rendering_error')}>
            {t('something_went_wrong_rendering_pdf')}
            &nbsp;
            <Trans
              i18nKey="try_recompile_project_or_troubleshoot"
              components={[<a href={TROUBLESHOOTING_URL} target="_blank" key="troubleshooting-link" />]}
            />
          </ErrorLogEntry>
        )
      )

    case 'clsi-maintenance':
      return includeErrors && <ErrorLogEntry title={t('server_error')}>{t('clsi_maintenance')}</ErrorLogEntry>

    case 'clsi-unavailable':
      return includeErrors && <ErrorLogEntry title={t('server_error')}>{t('clsi_unavailable')}</ErrorLogEntry>

    case 'too-recently-compiled':
      return includeErrors && <ErrorLogEntry title={t('server_error')}>{t('too_recently_compiled')}</ErrorLogEntry>

    case 'terminated':
      return includeErrors && <ErrorLogEntry title={t('terminated')}>{t('compile_terminated_by_user')}</ErrorLogEntry>

    case 'rate-limited':
      return (
        includeErrors && <ErrorLogEntry title={t('pdf_compile_rate_limit_hit')}>{t('project_flagged_too_many_compiles')}</ErrorLogEntry>
      )

    case 'compile-in-progress':
      return includeErrors && <ErrorLogEntry title={t('pdf_compile_in_progress_error')}>{t('pdf_compile_try_again')}</ErrorLogEntry>

    case 'autocompile-disabled':
      return includeErrors && <ErrorLogEntry title={t('autocompile_disabled')}>{t('autocompile_disabled_reason')}</ErrorLogEntry>

    case 'project-too-large':
      return includeErrors && <ErrorLogEntry title={t('project_too_large')}>{t('project_too_much_editable_text')}</ErrorLogEntry>

    case 'timedout':
      return includeErrors && <TimedOutLogEntry />

    case 'failure':
      return (
        includeErrors && (
          <ErrorLogEntry title={t('no_pdf_error_title')}>
            {t('no_pdf_error_explanation')}

            <ul className="my-1 ps-3">
              <li>{t('no_pdf_error_reason_unrecoverable_error')}</li>
              <li>
                <Trans i18nKey="no_pdf_error_reason_no_content" components={{ code: <code /> }} />
              </li>
              <li>
                <Trans i18nKey="no_pdf_error_reason_output_pdf_already_exists" components={{ code: <code /> }} />
              </li>
            </ul>
          </ErrorLogEntry>
        )
      )

    case 'clear-cache':
      return includeErrors && <ErrorLogEntry title={t('server_error')}>{t('somthing_went_wrong_compiling')}</ErrorLogEntry>

    case 'pdf-viewer-loading-error':
      return (
        includeErrors && (
          <ErrorLogEntry title={t('pdf_rendering_error')}>
            <Trans
              i18nKey="something_went_wrong_loading_pdf_viewer"
              components={[
                <strong key="strong-" />,
                <a href={TROUBLESHOOTING_URL} target="_blank" key="troubleshooting-link" />,
                <a key="contact-link" target="_blank" href="/contact" />,
              ]}
            />
          </ErrorLogEntry>
        )
      )

    case 'validation-problems':
      return null // handled elsewhere

    case 'error':
    default:
      return includeErrors && <ErrorLogEntry title={t('server_error')}>{t('somthing_went_wrong_compiling')}</ErrorLogEntry>
  }
}

export default memo(PdfPreviewError)

function ErrorLogEntry({ autoExpand = true, title, children }: { autoExpand?: boolean; title: string; children: ReactNode }) {
  const { t } = useTranslation()

  return (
    <PdfLogEntry
      autoExpand={autoExpand}
      headerTitle={title}
      formattedContent={children}
      entryAriaLabel={t('compile_error_entry_description')}
      level="error"
    />
  )
}

function TimedOutLogEntry() {
  const { t } = useTranslation()
  const { enableStopOnFirstError } = useStopOnFirstError()
  const { startCompile, lastCompileOptions, setAnimateCompileDropdownArrow } = useCompile()

  const handleEnableStopOnFirstErrorClick = useCallback(() => {
    enableStopOnFirstError()
    startCompile({ stopOnFirstError: true })
    setAnimateCompileDropdownArrow(true)
  }, [enableStopOnFirstError, startCompile, setAnimateCompileDropdownArrow])

  return (
    <ErrorLogEntry autoExpand title={t('timedout')}>
      <p>{t('project_timed_out_intro')}</p>
      <ul>
        <li>
          <Trans
            i18nKey="project_timed_out_optimize_images"
            components={[<a key="optimize" href="https://www.overleaf.com/learn/how-to/Optimising_very_large_image_files" />]}
          />
        </li>
        <li>
          <Trans
            i18nKey="project_timed_out_fatal_error"
            components={[
              <a
                key="fatal-error"
                href="https://www.overleaf.com/learn/how-to/Why_do_I_keep_getting_the_compile_timeout_error_message%3F#Fatal_compile_errors_blocking_the_compilation"
              />,
            ]}
          />
          {!lastCompileOptions.stopOnFirstError && (
            <>
              {' '}
              <Trans
                i18nKey="project_timed_out_enable_stop_on_first_error"
                components={[<Button key="enable" variant="primary" size="sm" onClick={handleEnableStopOnFirstErrorClick} />]}
              />{' '}
            </>
          )}
        </li>
      </ul>
      <p>
        <Trans
          i18nKey="project_timed_out_learn_more"
          components={[
            <a key="learn-more" href="https://www.overleaf.com/learn/how-to/Why_do_I_keep_getting_the_compile_timeout_error_message%3F" />,
          ]}
        />
      </p>
    </ErrorLogEntry>
  )
}
