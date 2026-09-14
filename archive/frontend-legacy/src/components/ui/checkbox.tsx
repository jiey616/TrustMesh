import { useId } from 'react'

interface CheckboxProps {
  checked?: boolean
  onCheckedChange?: (checked: boolean) => void
  disabled?: boolean
  id?: string
}

export function Checkbox({ checked = false, onCheckedChange, disabled = false, id }: CheckboxProps) {
  const autoId = useId()
  const checkboxId = id ?? autoId

  return (
    <input
      id={checkboxId}
      type="checkbox"
      checked={checked}
      disabled={disabled}
      onChange={(e) => onCheckedChange?.(e.target.checked)}
      className="size-4 rounded border-primary/30 text-primary accent-primary cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
    />
  )
}
