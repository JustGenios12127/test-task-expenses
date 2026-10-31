import { useEffect, useState } from 'react'
import {
  ConflictError,
  createExpense,
  deleteExpense,
  loadExpenses,
  loadMembers,
  updateExpense,
} from '../../api/expenses.ts'
import { expenseCount, periodCaption, periodRange } from '../../lib/expenses.ts'
import type { Category, Expense, ExpenseInput, Member, Period } from '../../shared/types.ts'
import { ExpenseConfirm } from './components/ExpenseConfirm.tsx'
import { ExpenseFilters } from './components/ExpenseFilters.tsx'
import { ExpenseForm } from './components/ExpenseForm.tsx'
import { ExpenseList } from './components/ExpenseList.tsx'
import { ExpenseTotal } from './components/ExpenseTotal.tsx'

type LoadStatus = 'loading' | 'ready' | 'error'
type Editor =
  | { kind: 'create' }
  | { kind: 'edit'; expense: Expense }
  | { kind: 'delete'; expense: Expense }
  | null

export default function Home() {
  const [expenses, setExpenses] = useState<Expense[]>([])
  const [total, setTotal] = useState(0)
  const [count, setCount] = useState(0)
  const [members, setMembers] = useState<Member[]>([])
  const [status, setStatus] = useState<LoadStatus>('loading')
  const [attempt, setAttempt] = useState(0)
  const [memberId, setMemberId] = useState<number | 'all'>('all')
  const [period, setPeriod] = useState<Period>('month')
  const [category, setCategory] = useState<Category | 'all'>('all')
  const [editor, setEditor] = useState<Editor>(null)
  const [pending, setPending] = useState(false)
  const [actionError, setActionError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    const range = periodRange(period)
    Promise.all([
      loadExpenses(
        { memberId, category, from: range?.from ?? '', to: range?.to ?? '' },
        controller.signal,
      ),
      loadMembers(controller.signal),
    ])
      .then(([page, people]) => {
        setExpenses(page.expenses)
        setTotal(page.total)
        setCount(page.count)
        setMembers(mergeMembers(people, page.expenses))
        setStatus('ready')
      })
      .catch(() => {
        if (controller.signal.aborted) {
          return
        }
        setStatus('error')
      })
    return () => controller.abort()
  }, [attempt, memberId, period, category])

  const emptyLabel =
    memberId === 'all' && category === 'all' && period === 'all'
      ? 'В базе пока нет трат.'
      : 'За этот отбор трат нет. Смените члена семьи, период или категорию.'

  function openCreate() {
    setActionError('')
    setEditor({ kind: 'create' })
  }

  function openEdit(expense: Expense) {
    setActionError('')
    setEditor({ kind: 'edit', expense })
  }

  function openDelete(expense: Expense) {
    setActionError('')
    setEditor({ kind: 'delete', expense })
  }

  function closeEditor() {
    if (pending) {
      return
    }
    setEditor(null)
    setActionError('')
  }

  function refresh() {
    setAttempt((value) => value + 1)
  }

  async function save(input: ExpenseInput) {
    if (!editor || editor.kind === 'delete') {
      return
    }
    setPending(true)
    setActionError('')
    setNotice('')
    try {
      if (editor.kind === 'create') {
        await createExpense(input)
      } else {
        await updateExpense(editor.expense.id, input, editor.expense.updatedAt)
      }
      setEditor(null)
      refresh()
    } catch (error) {
      if (error instanceof ConflictError) {
        setEditor(null)
        setNotice('Эту трату успели изменить, пока была открыта форма. Список обновлён, откройте трату ещё раз.')
        refresh()
        return
      }
      setActionError(error instanceof Error ? error.message : 'Не удалось сохранить трату')
    } finally {
      setPending(false)
    }
  }

  async function remove() {
    if (!editor || editor.kind !== 'delete') {
      return
    }
    setPending(true)
    setActionError('')
    setNotice('')
    try {
      await deleteExpense(editor.expense.id)
      setEditor(null)
      refresh()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Не удалось удалить трату')
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="flex h-full items-center px-6 py-10 sm:px-10">
      <div className="mx-auto grid w-full max-w-6xl items-start gap-10 lg:grid-cols-2 lg:gap-0">
        <section className="flex flex-col gap-8 lg:pr-14">
          <div className="flex flex-col gap-4">
            <h1 className="text-[28px] leading-none font-semibold tracking-[-0.02em] text-white">
              Расходы
            </h1>
            {status === 'ready' ? (
              <ExpenseTotal
                amount={total}
                periodLabel={periodCaption(period)}
                countLabel={expenseCount(count)}
              />
            ) : (
              <p className="text-base leading-6 text-white/60">
                {status === 'loading' ? 'Загружаю траты' : 'База не ответила'}
              </p>
            )}
          </div>
          <ExpenseFilters
            members={members}
            memberId={memberId}
            period={period}
            category={category}
            onMemberChange={setMemberId}
            onPeriodChange={setPeriod}
            onCategoryChange={setCategory}
          />
        </section>
        <div className="min-w-0 lg:border-l lg:border-white/20 lg:pl-14">
          {status === 'loading' ? (
            <p className="border-y border-white/20 py-6 text-base leading-6 text-white/70">Загружаю траты.</p>
          ) : null}
          {status === 'error' ? (
            <div className="border-y border-white/20 py-6">
              <p className="text-base leading-6 text-white/70">Не удалось загрузить траты из базы.</p>
              <button
                type="button"
                onClick={() => {
                  setStatus('loading')
                  setAttempt((value) => value + 1)
                }}
                className="mt-4 inline-flex h-11 cursor-pointer items-center rounded-full border border-white px-4 text-[15px] text-white"
              >
                Повторить
              </button>
            </div>
          ) : null}
          {status === 'ready' ? (
            <div className="flex flex-col gap-4">
              <div>
                <button
                  type="button"
                  onClick={openCreate}
                  disabled={members.length === 0}
                  className="inline-flex h-11 cursor-pointer items-center rounded-full border border-white px-4 text-[15px] text-white disabled:cursor-default disabled:border-white/20 disabled:text-white/40"
                >
                  Добавить
                </button>
              </div>
              {notice ? <p className="text-[15px] leading-5 text-white">{notice}</p> : null}
              <ExpenseList
                expenses={expenses}
                emptyLabel={emptyLabel}
                onEdit={openEdit}
                onDelete={openDelete}
              />
              {count > expenses.length ? (
                <p className="text-[15px] leading-5 text-white/60">
                  Показаны последние {expenses.length} из {count}. Сузьте период или выберите члена семьи.
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
      {editor?.kind === 'create' || editor?.kind === 'edit' ? (
        <ExpenseForm
          title={editor.kind === 'create' ? 'Новая трата' : 'Изменить трату'}
          members={members}
          initial={editor.kind === 'create' ? blankInput(members) : inputFrom(editor.expense)}
          pending={pending}
          error={actionError}
          onClose={closeEditor}
          onSubmit={(input) => {
            void save(input)
          }}
        />
      ) : null}
      {editor?.kind === 'delete' ? (
        <ExpenseConfirm
          title={deleteTitle(editor.expense)}
          pending={pending}
          error={actionError}
          onClose={closeEditor}
          onConfirm={() => {
            void remove()
          }}
        />
      ) : null}
    </main>
  )
}

function mergeMembers(people: Member[], expenses: Expense[]): Member[] {
  const byId = new Map<number, Member>()
  for (const member of people) {
    byId.set(member.id, member)
  }
  for (const expense of expenses) {
    if (!byId.has(expense.userId)) {
      byId.set(expense.userId, { id: expense.userId, name: expense.name })
    }
  }
  return [...byId.values()].sort((left, right) => left.name.localeCompare(right.name, 'ru'))
}

function blankInput(members: Member[]): ExpenseInput {
  return {
    userId: members[0]?.id ?? 0,
    amount: '',
    category: 'еда',
    spentAt: todayISO(),
    comment: '',
  }
}

function inputFrom(expense: Expense): ExpenseInput {
  return {
    userId: expense.userId,
    amount: amountInput(expense.amount),
    category: expense.category,
    spentAt: expense.spentAt,
    comment: expense.comment,
  }
}

function amountInput(amount: number): string {
  const kopecks = Math.round(amount * 100)
  const whole = Math.trunc(Math.abs(kopecks) / 100)
  const frac = Math.abs(kopecks) % 100
  if (frac === 0) {
    return String(whole)
  }
  return `${whole}.${String(frac).padStart(2, '0')}`
}

function todayISO(): string {
  const now = new Date()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}

function deleteTitle(expense: Expense): string {
  const name = expense.comment || expense.category
  return `Удалить «${name}»?`
}
