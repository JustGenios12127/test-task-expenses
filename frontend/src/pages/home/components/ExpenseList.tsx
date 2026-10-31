import { formatRubles, formatSpentAt } from '../../../lib/expenses.ts'
import type { Expense } from '../../../shared/types.ts'

type ExpenseListProps = {
  expenses: Expense[]
  emptyLabel: string
  onEdit: (expense: Expense) => void
  onDelete: (expense: Expense) => void
}

export function ExpenseList({ expenses, emptyLabel, onEdit, onDelete }: ExpenseListProps) {
  if (expenses.length === 0) {
    return (
      <p className="border-y border-white/20 py-6 text-base leading-6 text-white/70">{emptyLabel}</p>
    )
  }

  return (
    <ul className="border-y border-white/20">
      {expenses.map((expense) => {
        const title = expense.comment || expense.category
        return (
          <li
            key={expense.id}
            className="flex items-start justify-between gap-6 border-t border-white/20 py-2.5 first:border-t-0"
          >
            <div className="min-w-0">
              <p className="text-[17px] leading-6 text-white">{title}</p>
              <p className="mt-0.5 flex flex-wrap gap-x-4 text-[15px] leading-5 text-white/60">
                {expense.comment ? <span>{expense.category}</span> : null}
                <span>{expense.name}</span>
                <time dateTime={expense.spentAt}>{formatSpentAt(expense.spentAt)}</time>
              </p>
              <div className="mt-2 flex gap-2">
                <button
                  type="button"
                  onClick={() => onEdit(expense)}
                  className="inline-flex h-9 cursor-pointer items-center text-[15px] text-white/70 hover:text-white"
                >
                  Изменить
                </button>
                <button
                  type="button"
                  onClick={() => onDelete(expense)}
                  className="inline-flex h-9 cursor-pointer items-center text-[15px] text-white/70 hover:text-white"
                >
                  Удалить
                </button>
              </div>
            </div>
            <p className="shrink-0 text-[17px] leading-6 font-medium text-white tabular-nums">
              {formatRubles(expense.amount)}
            </p>
          </li>
        )
      })}
    </ul>
  )
}
