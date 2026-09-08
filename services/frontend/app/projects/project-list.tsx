'use client'

import {
  Button,
  Input,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from '@heroui/react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useMemo, useState, type FormEvent } from 'react'
import { FormError } from '@/components/form-error'
import { createProject, type ProjectSummary } from '@/lib/projects'

/**
 * Somebody's projects.
 *
 * The list arrives rendered from the server and is filtered here, because the
 * number of projects one person has is small enough that asking the server to
 * filter would be slower than not asking it.
 */
export function ProjectList({ projects }: { projects: ProjectSummary[] }) {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const live = projects.filter(project => !project.trashed && !project.archived)
    if (!needle) return live
    return live.filter(project => project.name.toLowerCase().includes(needle))
  }, [projects, query])

  async function create(event: FormEvent) {
    event.preventDefault()
    if (!name.trim()) return
    setError(null)
    setBusy(true)
    try {
      const project = await createProject(name.trim())
      router.push(`/projects/${project.id}`)
    } catch (thrown) {
      setError(thrown)
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
          <p className="text-small text-default-500">
            {visible.length} {visible.length === 1 ? 'project' : 'projects'}
          </p>
        </div>
        <form className="flex gap-2" onSubmit={create}>
          <Input
            aria-label="New project name"
            placeholder="New project"
            value={name}
            onValueChange={setName}
            size="sm"
            variant="bordered"
            className="w-56"
          />
          <Button type="submit" color="primary" size="sm" isLoading={busy}>
            Create
          </Button>
        </form>
      </div>

      <FormError error={error} />

      <Input
        aria-label="Search projects"
        placeholder="Search"
        value={query}
        onValueChange={setQuery}
        size="sm"
        variant="bordered"
        isClearable
        onClear={() => setQuery('')}
        className="max-w-sm"
      />

      {visible.length === 0 ? (
        <div className="rounded-large border border-dashed border-default-200 px-6 py-16 text-center">
          <p className="text-default-500">
            {query ? 'Nothing matches that.' : 'No projects yet.'}
          </p>
        </div>
      ) : (
        <Table aria-label="Projects" removeWrapper>
          <TableHeader>
            <TableColumn>Name</TableColumn>
            <TableColumn>Access</TableColumn>
            <TableColumn>Last updated</TableColumn>
          </TableHeader>
          <TableBody>
            {visible.map(project => (
              <TableRow key={project.id}>
                <TableCell>
                  <Link href={`/projects/${project.id}`} className="font-medium hover:underline">
                    {project.name}
                  </Link>
                </TableCell>
                <TableCell className="text-default-500">
                  {project.access === 'owner' ? 'Owner' : project.access === 'readOnly' ? 'Read only' : 'Editor'}
                </TableCell>
                <TableCell className="text-default-500">
                  {formatWhen(project.lastUpdated)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

/** A date in the reader's own locale, decided in the browser so the server
 *  does not have to guess at one. */
function formatWhen(iso: string): string {
  if (!iso) return ''
  const when = new Date(iso)
  if (Number.isNaN(when.getTime())) return ''
  return when.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
