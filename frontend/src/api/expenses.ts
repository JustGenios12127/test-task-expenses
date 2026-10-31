import { categories } from '../shared/constants.ts'
import type { Category, Expense, ExpenseFilter, ExpenseInput, ExpensePage, Member } from '../shared/types.ts'

export class ConflictError extends Error {}

type RawExpense = {
  id: number
  userId: number
  name: string
  amount: string
  category: string
  spentAt: string
  comment: string
  createdAt: string
  updatedAt: string
}

export async function loadExpenses(filter: ExpenseFilter, signal: AbortSignal): Promise<ExpensePage> {
  const params = new URLSearchParams()
  if (filter.memberId !== 'all') {
    params.set('userId', String(filter.memberId))
  }
  if (filter.category !== 'all') {
    params.set('category', filter.category)
  }
  if (filter.from) {
    params.set('from', filter.from)
  }
  if (filter.to) {
    params.set('to', filter.to)
  }
  const query = params.toString()
  const response = await fetch(query ? `/api/expenses?${query}` : '/api/expenses', { signal })
  if (!response.ok) {
    throw new Error('Не удалось загрузить траты')
  }

  const body: unknown = await response.json()
  if (!isPayload(body)) {
    throw new Error('Не удалось загрузить траты')
  }

  return {
    expenses: body.expenses.map(toExpense),
    total: parseAmount(body.total),
    count: body.count,
  }
}

export async function loadMembers(signal: AbortSignal): Promise<Member[]> {
  const response = await fetch('/api/members', { signal })
  if (!response.ok) {
    throw new Error('Не удалось загрузить членов семьи')
  }
  const body: unknown = await response.json()
  if (!isMemberPayload(body)) {
    throw new Error('Не удалось загрузить членов семьи')
  }
  return body.members.flatMap((item) => {
    if (!isMember(item)) {
      return []
    }
    return [{ id: item.id, name: item.name.trim() || 'без ника' }]
  })
}

export async function createExpense(input: ExpenseInput): Promise<void> {
  await changeExpense('/api/expenses', 'POST', input)
}

export async function updateExpense(id: number, input: ExpenseInput, updatedAt: string): Promise<void> {
  await changeExpense(`/api/expenses/${id}`, 'PUT', { ...input, updatedAt })
}

export async function deleteExpense(id: number): Promise<void> {
  const response = await fetch(`/api/expenses/${id}`, { method: 'DELETE' })
  if (!response.ok) {
    await rejectResponse(response, 'Не удалось удалить трату')
  }
}

async function changeExpense(
  url: string,
  method: 'POST' | 'PUT',
  input: ExpenseInput & { updatedAt?: string },
): Promise<void> {
  const response = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (response.status === 409) {
    throw new ConflictError('Трату уже изменили')
  }
  if (!response.ok) {
    await rejectResponse(response, 'Не удалось сохранить трату')
  }
}

async function rejectResponse(response: Response, fallback: string): Promise<never> {
  let message = fallback
  try {
    const body: unknown = await response.json()
    if (isErrorBody(body)) {
      message = body.error
    }
  } catch {
    message = fallback
  }
  throw new Error(message)
}

function isErrorBody(value: unknown): value is { error: string } {
  return typeof value === 'object' && value !== null && 'error' in value && typeof value.error === 'string' && value.error !== ''
}

function isMemberPayload(value: unknown): value is { members: unknown[] } {
  return typeof value === 'object' && value !== null && 'members' in value && Array.isArray(value.members)
}

function isMember(value: unknown): value is { id: number; name: string } {
  if (typeof value !== 'object' || value === null) {
    return false
  }
  const item = value as Record<string, unknown>
  return typeof item.id === 'number' && typeof item.name === 'string'
}

function isPayload(value: unknown): value is { expenses: unknown[]; total: string; count: number } {
  if (typeof value !== 'object' || value === null) {
    return false
  }
  const body = value as Record<string, unknown>
  return Array.isArray(body.expenses) && typeof body.total === 'string' && typeof body.count === 'number'
}

function toExpense(value: unknown): Expense {
  if (!isRawExpense(value) || !isCategory(value.category)) {
    throw new Error('Не удалось загрузить траты')
  }
  return {
    id: value.id,
    userId: value.userId,
    name: value.name.trim() || 'без ника',
    amount: parseAmount(value.amount),
    category: value.category,
    spentAt: value.spentAt,
    comment: value.comment,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt,
  }
}

function isRawExpense(value: unknown): value is RawExpense {
  if (typeof value !== 'object' || value === null) {
    return false
  }
  const item = value as Record<string, unknown>
  return (
    typeof item.id === 'number' &&
    typeof item.userId === 'number' &&
    typeof item.name === 'string' &&
    typeof item.amount === 'string' &&
    typeof item.category === 'string' &&
    typeof item.spentAt === 'string' &&
    typeof item.comment === 'string' &&
    typeof item.createdAt === 'string' &&
    typeof item.updatedAt === 'string'
  )
}

function isCategory(value: string): value is Category {
  return (categories as readonly string[]).includes(value)
}

function parseAmount(value: string): number {
  const [wholeText, fracText = ''] = value.split('.')
  if (!/^\d+$/.test(wholeText) || (fracText !== '' && !/^\d{1,2}$/.test(fracText))) {
    throw new Error('Не удалось загрузить траты')
  }
  const whole = Number(wholeText)
  const frac = Number((fracText + '00').slice(0, 2))
  return whole + frac / 100
}
