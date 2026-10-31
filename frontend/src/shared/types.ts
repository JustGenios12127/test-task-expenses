import type { categories, periods } from './constants.ts'

export type Category = (typeof categories)[number]

export type Period = (typeof periods)[number]['value']

export type Member = {
  id: number
  name: string
}

export type Expense = {
  id: number
  userId: number
  name: string
  amount: number
  category: Category
  spentAt: string
  comment: string
  createdAt: string
  updatedAt: string
}

export type ExpenseFilter = {
  memberId: number | 'all'
  category: Category | 'all'
  from: string
  to: string
}

export type ExpensePage = {
  expenses: Expense[]
  total: number
  count: number
}

export type ExpenseInput = {
  userId: number
  amount: string
  category: Category
  spentAt: string
  comment: string
}
