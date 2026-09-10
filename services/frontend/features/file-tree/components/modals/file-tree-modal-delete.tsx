'use client'

import { useTranslation } from '@/lib/i18n'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { Notification } from '@/components/ol/notification'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'

/** Confirms a delete, naming everything that is about to go. */
export function FileTreeModalDelete() {
  const { t } = useTranslation()

  const { isDeleting, inFlight, finishDeleting, actionedEntities, cancel, error } = useFileTreeActionable()

  if (!isDeleting) {
    return null
  }

  return (
    <OLModal show onHide={cancel}>
      <OLModalHeader>
        <OLModalTitle>{t('delete')}</OLModalTitle>
      </OLModalHeader>

      <OLModalBody>
        <p>{t('sure_you_want_to_delete')}</p>
        <ul>
          {actionedEntities?.map(entity => (
            <li key={entity.id}>{entity.name}</li>
          ))}
        </ul>
        {error != null && <Notification type="error" content={t('generic_something_went_wrong')} />}
      </OLModalBody>

      <OLModalFooter>
        {inFlight ? (
          <Button variant="danger" disabled isLoading loadingLabel={t('deleting')} />
        ) : (
          <>
            <Button variant="secondary" onClick={cancel}>
              {t('cancel')}
            </Button>
            <Button variant="danger" onClick={() => void finishDeleting()}>
              {t('delete')}
            </Button>
          </>
        )}
      </OLModalFooter>
    </OLModal>
  )
}

export default FileTreeModalDelete
