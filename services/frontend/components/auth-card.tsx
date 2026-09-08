import { Card, CardBody, CardHeader } from '@heroui/react'
import type { ReactNode } from 'react'

/**
 * The frame the sign-in and sign-up pages share.
 *
 * They are the same shape and differ only in what is inside, so the shape
 * lives here: making one of them wider later should not be possible by
 * accident.
 */
export function AuthCard({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string
  subtitle?: ReactNode
  children: ReactNode
  footer?: ReactNode
}) {
  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        <Card className="p-2" shadow="sm">
          <CardHeader className="flex flex-col items-start gap-1 px-6 pt-6">
            <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
            {subtitle ? (
              <div className="text-small text-default-500">{subtitle}</div>
            ) : null}
          </CardHeader>
          <CardBody className="gap-4 px-6 pb-6">{children}</CardBody>
        </Card>
        {footer ? (
          <div className="mt-6 text-center text-small text-default-500">{footer}</div>
        ) : null}
      </div>
    </main>
  )
}
