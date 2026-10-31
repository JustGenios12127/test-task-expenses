import { useEffect, useId, useState, type FormEvent } from 'react'
import { categories } from '../../../shared/constants.ts'
import type { Category, ExpenseInput, Member } from '../../../shared/types.ts'

type ExpenseFormProps = {
  title: string
  members: Member[]
  initial: ExpenseInput
  pending: boolean
  error: string
  onClose: () => void
  onSubmit: (input: ExpenseInput) => void
}

const fieldClass =
  'h-11 w-full rounded-xl border border-white/20 bg-black px-3 text-[15px] text-white outline-none'

export function ExpenseForm({
  title,
  members,
  initial,
  pending,
  error,
  onClose,
  onSubmit,
}: ExpenseFormProps) {
  const titleId = useId()
  const [draft, setDraft] = useState(initial)
  const [localError, setLocalError] = useState('')

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape' && !pending) {
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, pending])

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (draft.userId <= 0) {
      setLocalError('Выберите члена семьи')
      return
    }
    if (draft.amount.trim() === '') {
      setLocalError('Укажите сумму')
      return
    }
    if (draft.spentAt === '') {
      setLocalError('Укажите дату')
      return
    }
    setLocalError('')
    onSubmit({
      ...draft,
      amount: draft.amount.trim().replace(',', '.'),
      comment: draft.comment.trim(),
    })
  }

  const message = localError || error

  return (
    <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/80 px-4" onClick={pending ? undefined : onClose}>
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(event) => event.stopPropagation()}
        onSubmit={submit}
        className="w-full max-w-md border border-white/20 bg-black p-6"
      >
        <h2 id={titleId} className="text-[22px] leading-none font-semibold tracking-[-0.02em] text-white">
          {title}
        </h2>
        <div className="mt-6 flex flex-col gap-4">
          <label className="block">
            <span className="mb-2 block text-[15px] leading-5 text-white/60">Кто</span>
            <select
              value={draft.userId || ''}
              onChange={(event) => setDraft((current) => ({ ...current, userId: Number(event.target.value) }))}
              className={fieldClass}
            >
              {members.length === 0 ? <option value="">Нет членов семьи</option> : null}
              {members.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-2 block text-[15px] leading-5 text-white/60">Сумма</span>
            <input
              value={draft.amount}
              onChange={(event) => setDraft((current) => ({ ...current, amount: event.target.value }))}
              inputMode="decimal"
              autoFocus
              className={fieldClass}
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-[15px] leading-5 text-white/60">Категория</span>
            <select
              value={draft.category}
              onChange={(event) =>
                setDraft((current) => ({ ...current, category: event.target.value as Category }))
              }
              className={fieldClass}
            >
              {categories.map((category) => (
                <option key={category} value={category}>
                  {category}
                </option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-2 block text-[15px] leading-5 text-white/60">Дата</span>
            <input
              type="date"
              value={draft.spentAt}
              onChange={(event) => setDraft((current) => ({ ...current, spentAt: event.target.value }))}
              className={fieldClass}
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-[15px] leading-5 text-white/60">Комментарий</span>
            <input
              value={draft.comment}
              onChange={(event) => setDraft((current) => ({ ...current, comment: event.target.value }))}
              className={fieldClass}
            />
          </label>
        </div>
        {message ? <p className="mt-4 text-[15px] leading-5 text-white">{message}</p> : null}
        <div className="mt-6 flex gap-2">
          <button
            type="submit"
            disabled={pending || members.length === 0}
            className="inline-flex h-11 cursor-pointer items-center rounded-full border border-white px-4 text-[15px] text-white disabled:cursor-default disabled:border-white/20 disabled:text-white/40"
          >
            {pending ? 'Сохраняю' : 'Сохранить'}
          </button>
          <button
            type="button"
            onClick={onClose}
            disabled={pending}
            className="inline-flex h-11 cursor-pointer items-center rounded-full border border-white/20 px-4 text-[15px] text-white/70 disabled:cursor-default"
          >
            Отмена
          </button>
        </div>
      </form>
    </div>
  )
}
