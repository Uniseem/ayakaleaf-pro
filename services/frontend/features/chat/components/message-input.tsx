'use client'

import { useTranslation } from '@/lib/i18n'

/**
 * The box at the foot of the chat.
 *
 * Enter sends and Shift+Enter does not, except while an input method is
 * composing: pressing Enter to choose a Chinese or Japanese candidate would
 * otherwise send the half-finished word.
 */
export function MessageInput({
  resetUnreadMessages,
  sendMessage,
}: {
  resetUnreadMessages: () => void
  sendMessage: (message: string) => void
}) {
  const { t } = useTranslation()

  function handleKeyDown(event: React.KeyboardEvent) {
    const selectingCharacter = event.nativeEvent.isComposing
    if (event.key === 'Enter' && !event.shiftKey && !selectingCharacter) {
      event.preventDefault()
      const target = event.target as HTMLTextAreaElement
      sendMessage(target.value)
      // Deferred, so an input method has finished with the field before it is
      // cleared out from under it.
      window.setTimeout(() => {
        target.blur()
        target.closest('form')?.reset()
        target.focus()
      }, 0)
    }
  }

  return (
    <form className="new-message" onSubmit={event => event.preventDefault()}>
      <label htmlFor="chat-input" className="visually-hidden">
        {`${t('your_message_to_collaborators')}…`}
      </label>
      <textarea
        id="chat-input"
        placeholder={`${t('your_message_to_collaborators')}…`}
        onKeyDown={handleKeyDown}
        onClick={resetUnreadMessages}
      />
    </form>
  )
}

export default MessageInput
