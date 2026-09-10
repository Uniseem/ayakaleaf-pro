'use client'

/**
 * Bringing a project that already exists.
 *
 * A zip file, which is what everything else exports: a journal's template, a
 * download from somewhere else, a folder somebody compressed and emailed. The
 * server unpacks it, works out which file is the one to compile, and takes the
 * project's name from that file's title if it has one.
 *
 * A drop target as well as a button, because the thing being uploaded is
 * already in a window somewhere and dragging it is one movement instead of
 * four. The button stays for anybody who cannot drag, and for the fact that a
 * drop target with no visible control looks like decoration.
 */

import { useCallback, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import {
  OLModal,
  OLModalBody,
  OLModalFooter,
  OLModalHeader,
  OLModalTitle,
} from '@/components/ol/modal'
import { Button } from '@/components/ui'
import { messageFor } from '@/lib/api'
import { uploadProject } from '@/lib/projects'

export function UploadProjectModal({
  isOpen,
  onClose,
  onUploaded,
}: {
  isOpen: boolean
  onClose: () => void
  /** Told when a project has been made, so the list can catch up. */
  onUploaded: () => Promise<void> | void
}) {
  const router = useRouter()
  const input = useRef<HTMLInputElement>(null)

  const [over, setOver] = useState(false)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState<string | null>(null)

  const upload = useCallback(
    async (file: File) => {
      if (busy) {
        return
      }
      // Checked here as well as on the server: a person who picked the wrong
      // file should be told before waiting for it to upload.
      if (!/\.zip$/i.test(file.name)) {
        setError('That is not a zip file. Compress the project folder first.')
        return
      }
      setError(null)
      setBusy(true)
      setProgress(0)
      try {
        const project = await uploadProject(file, {
          onProgress: setProgress,
        })
        await onUploaded()
        // Straight into it. Somebody who has just uploaded a project wants to
        // see whether it survived the trip.
        router.push(`/projects/${project.id}`)
      } catch (thrown) {
        setError(messageFor(thrown))
        setBusy(false)
      }
    },
    [busy, onUploaded, router]
  )

  return (
    <OLModal
      show={isOpen}
      onHide={() => {
        if (!busy) {
          onClose()
        }
      }}
    >
      <OLModalHeader closeButton={!busy}>
        <OLModalTitle>Upload project</OLModalTitle>
      </OLModalHeader>

      <OLModalBody>
        <div
          onDragOver={event => {
            event.preventDefault()
            setOver(true)
          }}
          onDragLeave={() => setOver(false)}
          onDrop={event => {
            event.preventDefault()
            setOver(false)
            const file = event.dataTransfer.files?.[0]
            if (file) {
              void upload(file)
            }
          }}
          className={`flex flex-col items-center justify-center gap-3 rounded-[8px] border-2 border-dashed px-6 py-10 text-center ${
            over
              ? 'border-[var(--bg-accent-01)] bg-[var(--bg-accent-03)]'
              : 'border-[var(--border-divider)]'
          }`}
        >
          {busy ? (
            <>
              <p className="text-[16px] leading-6 text-[var(--content-primary)]">
                Uploading…
              </p>
              <div
                role="progressbar"
                aria-valuenow={Math.round(progress * 100)}
                aria-valuemin={0}
                aria-valuemax={100}
                className="h-1.5 w-56 overflow-hidden rounded-full bg-[var(--bg-light-tertiary)]"
              >
                <div
                  className="h-full rounded-full bg-[var(--bg-accent-01)] transition-[width] duration-150"
                  style={{ width: `${Math.round(progress * 100)}%` }}
                />
              </div>
              <p className="text-[12px] leading-4 text-[var(--content-secondary)]">
                {progress >= 1
                  ? 'Unpacking the files…'
                  : `${Math.round(progress * 100)}%`}
              </p>
            </>
          ) : (
            <>
              <UploadIcon />
              <p className="text-[16px] leading-6 text-[var(--content-primary)]">
                Drag a .zip file here
              </p>
              <Button
                onClick={() => input.current?.click()}
                className="min-w-40"
              >
                Select a file
              </Button>
              <p className="max-w-sm text-[12px] leading-4 text-[var(--content-secondary)]">
                The file that declares a document class becomes the one that is
                compiled. Build output — .aux, .log, .bbl and the rest — is left
                out, because the compiler writes it again.
              </p>
            </>
          )}

          <input
            ref={input}
            type="file"
            accept=".zip,application/zip"
            className="hidden"
            onChange={event => {
              const file = event.target.files?.[0]
              // Cleared so that picking the same file twice, after an error,
              // is still a change.
              event.target.value = ''
              if (file) {
                void upload(file)
              }
            }}
          />
        </div>

        {error ? (
          <p
            role="alert"
            className="mt-3 rounded-[4px] bg-[var(--bg-danger-03)] px-3 py-2 text-[14px] leading-5 text-[var(--content-danger)]"
          >
            {error}
          </p>
        ) : null}
      </OLModalBody>

      <OLModalFooter>
        <Button kind="secondary" onClick={onClose} disabled={busy}>
          Cancel
        </Button>
      </OLModalFooter>
    </OLModal>
  )
}

function UploadIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="h-8 w-8 text-[var(--content-secondary)]"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      aria-hidden
    >
      <path d="M12 16V4m0 0L8 8m4-4 4 4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M4 16v2.5A1.5 1.5 0 0 0 5.5 20h13a1.5 1.5 0 0 0 1.5-1.5V16" strokeLinecap="round" />
    </svg>
  )
}
