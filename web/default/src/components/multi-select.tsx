/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import * as React from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Command, CommandGroup, CommandItem } from '@/components/ui/command'

export type Option = {
  label: string
  value: string
}

interface MultiSelectProps {
  options: Option[]
  selected: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  className?: string
  allowCustomValues?: boolean
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  className,
  allowCustomValues = false,
}: MultiSelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder ?? t('Select items...')
  const inputRef = React.useRef<HTMLInputElement>(null)
  const [open, setOpen] = React.useState(false)
  const [inputValue, setInputValue] = React.useState('')

  const parseCustomValues = (value: string) =>
    value
      .split(/[\n,]+/)
      .map((item) => item.trim())
      .filter(Boolean)

  const addSelectedValues = (values: string[]) => {
    const next = [...selected]
    for (const value of values) {
      const normalized = value.trim()
      if (!normalized || next.includes(normalized)) {
        continue
      }
      next.push(normalized)
    }
    if (next.length !== selected.length) {
      onChange(next)
    }
  }

  const commitInputValue = () => {
    if (!allowCustomValues) {
      return false
    }
    const values = parseCustomValues(inputValue)
    if (values.length === 0) {
      return false
    }
    addSelectedValues(values)
    setInputValue('')
    return true
  }

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    if (!allowCustomValues) {
      return
    }
    const values = parseCustomValues(e.clipboardData.getData('text'))
    if (values.length <= 1) {
      return
    }
    e.preventDefault()
    addSelectedValues(values)
    setInputValue('')
  }

  const handleUnselect = (value: string) => {
    onChange(selected.filter((s) => s !== value))
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const input = inputRef.current
    if (input) {
      if (
        allowCustomValues &&
        (e.key === 'Enter' || e.key === 'Tab' || e.key === ',') &&
        input.value.trim() !== ''
      ) {
        e.preventDefault()
        commitInputValue()
        return
      }
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (input.value === '' && selected.length > 0) {
          onChange(selected.slice(0, -1))
        }
      }
      if (e.key === 'Escape') {
        input.blur()
      }
    }
  }

  const selectables = options.filter(
    (option) => !selected.includes(option.value)
  )

  return (
    <Command
      onKeyDown={handleKeyDown}
      className={`overflow-visible bg-transparent ${className || ''}`}
    >
      <div className='group border-input ring-offset-background focus-within:ring-ring rounded-md border px-3 py-2 text-sm focus-within:ring-2 focus-within:ring-offset-2'>
        <div className='flex flex-wrap gap-1'>
          {selected.map((value) => {
            const option = options.find((o) => o.value === value)
            return (
              <Badge key={value} variant='secondary'>
                {option?.label || value}
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label='Remove'
                  className='ml-1 size-auto p-0'
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      handleUnselect(value)
                    }
                  }}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                  }}
                  onClick={() => handleUnselect(value)}
                >
                  <X
                    className='text-muted-foreground hover:text-foreground h-3 w-3'
                    aria-hidden='true'
                  />
                </Button>
              </Badge>
            )
          })}
          <CommandPrimitive.Input
            ref={inputRef}
            value={inputValue}
            onValueChange={setInputValue}
            onPaste={handlePaste}
            onBlur={() => {
              commitInputValue()
              setOpen(false)
            }}
            onFocus={() => setOpen(true)}
            placeholder={selected.length === 0 ? resolvedPlaceholder : ''}
            className='placeholder:text-muted-foreground flex-1 bg-transparent outline-none'
          />
        </div>
      </div>
      <div className='relative'>
        {open && selectables.length > 0 ? (
          <div className='bg-popover text-popover-foreground animate-in absolute top-0 z-10 w-full rounded-md border shadow-md outline-none'>
            <CommandGroup className='h-full max-h-60 overflow-auto'>
              {selectables.map((option) => {
                return (
                  <CommandItem
                    key={option.value}
                    onMouseDown={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                    }}
                    onSelect={() => {
                      setInputValue('')
                      onChange([...selected, option.value])
                    }}
                    className='cursor-pointer'
                  >
                    {option.label}
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </div>
        ) : null}
      </div>
    </Command>
  )
}
