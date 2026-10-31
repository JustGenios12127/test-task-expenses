import { useId } from 'react'
import { categories, periods } from '../../../shared/constants.ts'
import type { Category, Member, Period } from '../../../shared/types.ts'

type ExpenseFiltersProps = {
  members: Member[]
  memberId: number | 'all'
  period: Period
  category: Category | 'all'
  onMemberChange: (memberId: number | 'all') => void
  onPeriodChange: (period: Period) => void
  onCategoryChange: (category: Category | 'all') => void
}

type FilterOption<T extends string | number> = {
  value: T
  label: string
}

const selectedClass =
  'inline-flex h-11 cursor-pointer items-center rounded-full border border-white px-4 text-[15px] leading-none text-white transition-colors duration-150'

const idleClass =
  'inline-flex h-11 cursor-pointer items-center rounded-full border border-white/20 px-4 text-[15px] leading-none text-white/55 transition-colors duration-150 hover:border-white/50 hover:text-white'

function FilterGroup<T extends string | number,>({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: T
  options: FilterOption<T>[]
  onChange: (value: T) => void
}) {
  const labelId = useId()

  return (
    <div>
      <p id={labelId} className="mb-2 text-[15px] leading-5 text-white/60">
        {label}
      </p>
      <div role="group" aria-labelledby={labelId} className="flex flex-wrap gap-2">
        {options.map((option) => {
          const selected = option.value === value
          return (
            <button
              key={String(option.value)}
              type="button"
              aria-pressed={selected}
              onClick={() => onChange(option.value)}
              className={selected ? selectedClass : idleClass}
            >
              {option.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

export function ExpenseFilters({
  members,
  memberId,
  period,
  category,
  onMemberChange,
  onPeriodChange,
  onCategoryChange,
}: ExpenseFiltersProps) {
  const memberOptions: FilterOption<number | 'all'>[] = [
    { value: 'all', label: 'Все' },
    ...members.map((member) => ({ value: member.id, label: member.name })),
  ]
  const periodOptions: FilterOption<Period>[] = periods.map((item) => ({
    value: item.value,
    label: item.label,
  }))
  const categoryOptions: FilterOption<Category | 'all'>[] = [
    { value: 'all', label: 'Все' },
    ...categories.map((item) => ({ value: item, label: item })),
  ]

  return (
    <div className="flex flex-col gap-6">
      <FilterGroup
        label="Кто"
        value={memberId}
        options={memberOptions}
        onChange={onMemberChange}
      />
      <FilterGroup
        label="Период"
        value={period}
        options={periodOptions}
        onChange={onPeriodChange}
      />
      <FilterGroup
        label="Категория"
        value={category}
        options={categoryOptions}
        onChange={onCategoryChange}
      />
    </div>
  )
}
