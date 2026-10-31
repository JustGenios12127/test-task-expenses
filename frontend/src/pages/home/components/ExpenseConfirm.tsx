import { useEffect, useId } from 'react'

type ExpenseConfirmProps = {
  title: string
  pending: boolean
  error: string
  onClose: () => void
  onConfirm: () => void
}

export function ExpenseConfirm({ title, pending, error, onClose, onConfirm }: ExpenseConfirmProps) {
  const titleId = useId()

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape' && !pending) {
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, pending])

  return (
    <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/80 px-4" onClick={pending ? undefined : onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(event) => event.stopPropagation()}
        className="w-full max-w-md border border-white/20 bg-black p-6"
      >
        <h2 id={titleId} className="text-[22px] leading-none font-semibold tracking-[-0.02em] text-white">
          Удалить трату
        </h2>
        <p className="mt-4 text-[17px] leading-6 text-white/70">{title}</p>
        {error ? <p className="mt-4 text-[15px] leading-5 text-white">{error}</p> : null}
        <div className="mt-6 flex gap-2">
          <button
            type="button"
            onClick={onConfirm}
            disabled={pending}
            className="inline-flex h-11 cursor-pointer items-center rounded-full border border-white px-4 text-[15px] text-white disabled:cursor-default disabled:border-white/20 disabled:text-white/40"
          >
            {pending ? 'Удаляю' : 'Удалить'}
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
      </div>
    </div>
  )
}
