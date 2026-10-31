import { formatRubles } from '../../../lib/expenses.ts'

type ExpenseTotalProps = {
  amount: number
  periodLabel: string
  countLabel: string
}

export function ExpenseTotal({ amount, periodLabel, countLabel }: ExpenseTotalProps) {
  return (
    <section aria-live="polite">
      <p className="text-[40px] leading-none font-semibold tracking-[-0.03em] text-white tabular-nums">
        {formatRubles(amount)}
      </p>
      <p className="mt-2 text-base leading-6 text-white/60">
        {countLabel} {periodLabel}
      </p>
    </section>
  )
}
