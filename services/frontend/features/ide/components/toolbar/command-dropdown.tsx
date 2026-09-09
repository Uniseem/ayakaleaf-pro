'use client'

/**
 * A menu built from the command registry, from
 * ide-react/components/toolbar/command-dropdown.tsx.
 *
 * The menu's structure is a list of sections of command ids; the commands
 * themselves come from whoever registered them. A section with nothing
 * registered is left out, and a menu with no sections is not shown at all.
 */

import { Fragment, useCallback, useMemo } from 'react'
import { DropdownDivider, DropdownHeader } from '@/components/ol/dropdown'
import { MenuBarDropdown, MenuBarOption, NestedMenuBarDropdown } from '@/components/ol/menu-bar'
import { formatShortcut, useCommandRegistry, type Command, type Shortcuts } from '@/features/ide/contexts/command-registry-context'

type CommandId = string
type TaggedCommand = Command & {
  type: 'command'
  shortcuts?: Shortcuts[CommandId]
}
type Entry<T> = T | GroupStructure<T>
type GroupStructure<T> = {
  id: string
  title: string
  children: Array<Entry<T>>
}
export type MenuSectionStructure<T = CommandId> = {
  title?: string
  id: string
  children: Array<Entry<T>>
}
export type MenuStructure<T = CommandId> = Array<MenuSectionStructure<T>>

export default function CommandDropdown({ menu, title, id }: { menu: MenuStructure<CommandId>; title: string; id: string }) {
  const { registry, shortcuts } = useCommandRegistry()
  const populatedSections = useMemo(
    () => menu.map(section => populateSectionOrGroup(section, registry, shortcuts)).filter(x => x.children.length > 0),
    [menu, registry, shortcuts]
  )

  if (populatedSections.length === 0) {
    return null
  }

  return (
    <MenuBarDropdown title={title} id={id} className="ide-redesign-toolbar-dropdown-toggle-subdued ide-redesign-toolbar-button-subdued">
      {populatedSections.map((section, index) => (
        <Fragment key={section.id}>
          <CommandSectionContent section={section} includeDivider={index > 0} />
        </Fragment>
      ))}
    </MenuBarDropdown>
  )
}

export function CommandSection({ section: sectionStructure, includeDivider = true }: { section: MenuSectionStructure<CommandId>; includeDivider?: boolean }) {
  const { registry, shortcuts } = useCommandRegistry()
  const section = populateSectionOrGroup(sectionStructure, registry, shortcuts)
  return <CommandSectionContent section={section} includeDivider={includeDivider} />
}

function CommandSectionContent({ section, includeDivider = true }: { section: MenuSectionStructure<TaggedCommand>; includeDivider?: boolean }) {
  if (section.children.length === 0) {
    return null
  }
  return (
    <>
      {includeDivider ? <DropdownDivider /> : null}
      {section.title ? <DropdownHeader>{section.title}</DropdownHeader> : null}
      {section.children.map(child => (
        <CommandDropdownChild item={child} key={child.id} />
      ))}
    </>
  )
}

function CommandDropdownChild({ item }: { item: Entry<TaggedCommand> }) {
  const onClickHandler = useCallback(() => {
    if (isTaggedCommand(item)) {
      item.handler?.({ location: 'menu-bar' })
    }
  }, [item])

  if (isTaggedCommand(item)) {
    return (
      <MenuBarOption
        eventKey={item.id}
        key={item.id}
        title={item.menuLabel ?? item.label}
        onClick={onClickHandler}
        href={item.href}
        disabled={item.disabled}
        leadingIcon={item.leadingIcon}
        trailingIcon={item.shortcuts && item.shortcuts[0] ? <span>{formatShortcut(item.shortcuts[0])}</span> : undefined}
      />
    )
  }
  return (
    <NestedMenuBarDropdown title={item.title} id={item.id} key={item.id}>
      {item.children.map(subChild => (
        <CommandDropdownChild item={subChild} key={subChild.id} />
      ))}
    </NestedMenuBarDropdown>
  )
}

function populateSectionOrGroup<T extends { children: Array<Entry<CommandId>> }>(
  section: T,
  registry: Map<string, Command>,
  shortcuts: Shortcuts
): Omit<T, 'children'> & { children: Array<Entry<TaggedCommand>> } {
  const { children, ...rest } = section
  const populated: Array<Entry<TaggedCommand>> = []
  for (const child of children) {
    if (typeof child !== 'string') {
      const populatedChild = populateSectionOrGroup(child, registry, shortcuts)
      if (populatedChild.children.length > 0) {
        populated.push(populatedChild as GroupStructure<TaggedCommand>)
      }
      continue
    }
    const command = registry.get(child)
    if (command) {
      populated.push({ ...command, shortcuts: shortcuts[command.id], type: 'command' as const })
    }
  }
  return { ...rest, children: populated }
}

function isTaggedCommand(item: Entry<TaggedCommand>): item is TaggedCommand {
  return 'type' in item && item.type === 'command'
}
