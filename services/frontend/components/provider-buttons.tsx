import { Button } from '@/components/ol/button'
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
        <hr className="flex-1 border-t border-[var(--border-divider)]" />
        <span className="text-xs uppercase tracking-wide text-[var(--content-secondary)]">or</span>
        <hr className="flex-1 border-t border-[var(--border-divider)]" />
      </div>
      {providers.map(provider => (
        <Button key={provider.id} href={provider.path} variant="secondary" className="w-full">
          Continue with {provider.name}
        </Button>
      ))}
    </div>
  )
}
