import { Button, Divider } from '@heroui/react'
import type { Provider } from '@/lib/auth'

/**
 * The identity providers an administrator configured.
 *
 * Nothing is drawn when none are: an "or" line above an empty space is what
 * the page it replaces did.
 */
export function ProviderButtons({ providers }: { providers: Provider[] }) {
  if (providers.length === 0) {
    return null
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <Divider className="flex-1" />
        <span className="text-tiny uppercase tracking-wide text-default-400">or</span>
        <Divider className="flex-1" />
      </div>
      {providers.map(provider => (
        <Button
          key={provider.id}
          as="a"
          href={provider.path}
          variant="bordered"
          fullWidth
        >
          Continue with {provider.name}
        </Button>
      ))}
    </div>
  )
}
