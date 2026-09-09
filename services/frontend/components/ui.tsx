'use client'

/**
 * The pieces the interface is made of.
 *
 * Written as plain elements against the design tokens rather than taken from a
 * component library, because the shapes here are specific: a button is a pill,
 * an input is a 4px rectangle with its label above it, and a sidebar item is
 * an 8px rounded row that turns green when it is the one you are on. Those are
 * measurements from the original, not preferences, and reproducing them by
 * overriding somebody else's defaults is more code than writing them.
 *
 * HeroUI is still used for the things that are behaviour rather than shape --
 * dropdowns, modals, tooltips -- where its look is themed to the same tokens.
 */

import Link from 'next/link'
import type {
  AnchorHTMLAttributes,
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
} from 'react'
import { forwardRef, useId } from 'react'

export type ButtonKind = 'primary' | 'secondary' | 'ghost' | 'danger' | 'link'
export type ButtonSize = 'sm' | 'md'

/**
 * A button.
 *
 * Pill-shaped at every size, which is the shape the original uses everywhere:
 * 36px tall at 16px/600 for a normal one, 24px at 14px/600 for the small ones
 * in the editor's toolbar.
 */
const kinds: Record<ButtonKind, string> = {
  primary:
    'bg-[var(--bg-accent-01)] text-white hover:bg-[var(--bg-accent-02)] ' +
    'disabled:bg-[var(--bg-light-disabled)] disabled:text-[var(--content-disabled)]',
  secondary:
    'bg-transparent text-[var(--content-primary)] border-2 border-[var(--border-primary)] ' +
    'hover:bg-[var(--hover-interaction)] disabled:border-[var(--border-disabled)] ' +
    'disabled:text-[var(--content-disabled)]',
  ghost:
    'bg-transparent text-[var(--content-primary)] hover:bg-[var(--hover-interaction)] ' +
    'disabled:text-[var(--content-disabled)]',
  danger:
    'bg-[var(--bg-danger-01)] text-white hover:bg-[var(--bg-danger-02)] ' +
    'disabled:bg-[var(--bg-light-disabled)] disabled:text-[var(--content-disabled)]',
  link:
    'bg-transparent text-[var(--link-web)] hover:text-[var(--link-web-hover)] ' +
    'hover:underline underline-offset-2 px-0',
}

const sizes: Record<ButtonSize, string> = {
  sm: 'h-6 px-3 text-[14px] leading-5',
  md: 'h-9 px-4 text-[16px] leading-6',
}

function buttonClass(kind: ButtonKind, size: ButtonSize, extra?: string) {
  return [
    'inline-flex items-center justify-center gap-1.5 rounded-full font-semibold',
    'transition-colors disabled:cursor-not-allowed whitespace-nowrap',
    kinds[kind],
    kind === 'link' ? 'h-auto font-normal' : sizes[size],
    extra ?? '',
  ].join(' ')
}

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  kind?: ButtonKind
  size?: ButtonSize
  loading?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button({ kind = 'primary', size = 'md', loading, className, children, disabled, ...rest }, ref) {
    return (
      <button
        ref={ref}
        type="button"
        disabled={disabled || loading}
        className={buttonClass(kind, size, className)}
        {...rest}
      >
        {loading ? <Spinner /> : null}
        {children}
      </button>
    )
  }
)

export type ButtonLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
  href: string
  kind?: ButtonKind
  size?: ButtonSize
}

export function ButtonLink({
  href,
  kind = 'primary',
  size = 'md',
  className,
  children,
  ...rest
}: ButtonLinkProps) {
  return (
    <Link href={href} className={buttonClass(kind, size, className)} {...rest}>
      {children}
    </Link>
  )
}

function Spinner() {
  return (
    <svg viewBox="0 0 16 16" className="h-3.5 w-3.5 animate-spin" aria-hidden>
      <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" strokeWidth="2" opacity="0.25" />
      <path d="M14 8a6 6 0 0 0-6-6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  )
}

/**
 * A text field, with its label above it.
 *
 * Above and not floating inside: the original puts it there, and a floating
 * label is a different thing to read past when the field already has a value.
 */
export type TextFieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label?: string
  hint?: string
  error?: string | null
}

export const TextField = forwardRef<HTMLInputElement, TextFieldProps>(
  function TextField({ label, hint, error, className, id, ...rest }, ref) {
    const generated = useId()
    const fieldId = id ?? generated
    return (
      <div className="flex flex-col gap-1">
        {label ? (
          <label
            htmlFor={fieldId}
            className="text-[14px] font-medium leading-5 text-[var(--content-primary)]"
          >
            {label}
          </label>
        ) : null}
        <input
          ref={ref}
          id={fieldId}
          className={[
            'w-full rounded-[4px] border bg-[var(--bg-light-primary)] px-2 py-1.5',
            'text-[16px] leading-6 text-[var(--content-primary)]',
            'placeholder:text-[var(--content-placeholder)]',
            'focus:border-[var(--border-active)] focus:outline-none',
            error ? 'border-[var(--border-danger)]' : 'border-[var(--border-primary)]',
            className ?? '',
          ].join(' ')}
          aria-invalid={error ? true : undefined}
          {...rest}
        />
        {error ? (
          <p className="text-[14px] leading-5 text-[var(--content-danger)]">{error}</p>
        ) : hint ? (
          <p className="text-[14px] leading-5 text-[var(--content-secondary)]">{hint}</p>
        ) : null}
      </div>
    )
  }
)

export type SelectFieldProps = SelectHTMLAttributes<HTMLSelectElement> & {
  label?: string
}

export const SelectField = forwardRef<HTMLSelectElement, SelectFieldProps>(
  function SelectField({ label, className, id, children, ...rest }, ref) {
    const generated = useId()
    const fieldId = id ?? generated
    return (
      <div className="flex flex-col gap-1">
        {label ? (
          <label
            htmlFor={fieldId}
            className="text-[14px] font-medium leading-5 text-[var(--content-primary)]"
          >
            {label}
          </label>
        ) : null}
        <select
          ref={ref}
          id={fieldId}
          className={[
            'w-full rounded-[4px] border border-[var(--border-primary)] bg-[var(--bg-light-primary)]',
            'px-2 py-1.5 text-[16px] leading-6 text-[var(--content-primary)]',
            'focus:border-[var(--border-active)] focus:outline-none',
            className ?? '',
          ].join(' ')}
          {...rest}
        >
          {children}
        </select>
      </div>
    )
  }
)

/** A message about the whole page or form. */
export function Notification({
  kind = 'info',
  children,
}: {
  kind?: 'info' | 'danger' | 'warning' | 'success'
  children: ReactNode
}) {
  const styles = {
    info: 'bg-[var(--bg-info-03)] text-[var(--content-info)]',
    danger: 'bg-[var(--bg-danger-03)] text-[var(--content-danger)]',
    warning: 'bg-[var(--bg-warning-03)] text-[var(--content-warning)]',
    success: 'bg-[var(--bg-accent-03)] text-[var(--content-positive)]',
  }[kind]
  return (
    <div role="alert" className={`rounded-[4px] px-4 py-3 text-[14px] leading-5 ${styles}`}>
      {children}
    </div>
  )
}

/** The grey panel the auth forms sit in. */
export function Card({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={[
        'rounded-[8px] bg-[var(--bg-light-secondary)] p-8',
        className ?? '',
      ].join(' ')}
    >
      {children}
    </div>
  )
}
