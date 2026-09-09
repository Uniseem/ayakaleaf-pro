'use client'

import { Fragment, useMemo } from 'react'
import MaterialIcon from '@/components/ol/material-icon'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useOutlineContext } from '@/features/ide/contexts/outline-context'
import { nestOutline, type Outline } from '../utils/tree-operations/outline'

const getChildrenLines = (children?: Outline[]): number[] =>
  (children || []).reduce<number[]>((lines, child) => lines.concat(getChildrenLines(child.children), child.line), [])

const constructOutlineHierarchy = (items: Outline[], highlightedLine: number, outlineHierarchy: Outline[] = []) => {
  for (const item of items) {
    if (item.line === highlightedLine) {
      outlineHierarchy.push(item)
      return outlineHierarchy
    }

    const childLines = getChildrenLines(item.children)
    if (childLines.includes(highlightedLine)) {
      outlineHierarchy.push(item)
      return constructOutlineHierarchy(item.children as Outline[], highlightedLine, outlineHierarchy)
    }
  }
  return outlineHierarchy
}

export default function Breadcrumbs() {
  const { current } = useEditor()
  const { flatOutline, highlightedLine, canShowOutline } = useOutlineContext()

  // the folders above the file, from its path
  const folderHierarchy = useMemo(() => {
    if (!current) {
      return []
    }
    const parts = current.path.replace(/^\//, '').split('/')
    return parts.slice(0, -1)
  }, [current])

  const fileName = current?.name

  const outline = useMemo(() => (flatOutline ? nestOutline(flatOutline.items) : []), [flatOutline])

  const outlineHierarchy = useMemo(() => {
    if (!current || !canShowOutline) {
      return []
    }

    return constructOutlineHierarchy(outline, highlightedLine)
  }, [outline, highlightedLine, canShowOutline, current])

  if (!current) {
    return null
  }

  const numOutlineItems = outlineHierarchy.length

  return (
    <div className="ol-cm-breadcrumbs" translate="no">
      {folderHierarchy.map((folder, index) => (
        <Fragment key={`${index}-${folder}`}>
          <div>{folder}</div>
          <Chevron />
        </Fragment>
      ))}
      <MaterialIcon unfilled type="description" />
      <div>{fileName}</div>
      {numOutlineItems > 0 && <Chevron />}
      {outlineHierarchy.map((section, idx) => (
        <Fragment key={section.line}>
          <div>{section.title}</div>
          {idx < numOutlineItems - 1 && <Chevron />}
        </Fragment>
      ))}
    </div>
  )
}

const Chevron = () => <MaterialIcon className="ol-cm-breadcrumb-chevron" type="chevron_right" />
