import type { Period } from '../shared/types.ts'

function startOfWeek(date: Date): Date {
  const day = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const weekday = day.getDay()
  const shift = weekday === 0 ? 6 : weekday - 1
  day.setDate(day.getDate() - shift)
  return day
}

function startOfMonth(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

function startOfYear(date: Date): Date {
  return new Date(date.getFullYear(), 0, 1)
}

function isoDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function periodRange(
  period: Period,
  today = new Date(),
): { from: string; to: string } | null {
  const day = new Date(today.getFullYear(), today.getMonth(), today.getDate())
  const to = isoDate(day)
  if (period === 'all') {
    return null
  }
  if (period === 'week') {
    return { from: isoDate(startOfWeek(day)), to }
  }
  if (period === 'month') {
    return { from: isoDate(startOfMonth(day)), to }
  }
  return { from: isoDate(startOfYear(day)), to }
}

export function formatRubles(amount: number): string {
  const kopecks = Math.round(amount * 100)
  const hasCents = Math.abs(kopecks) % 100 !== 0
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    minimumFractionDigits: hasCents ? 2 : 0,
    maximumFractionDigits: hasCents ? 2 : 0,
  }).format(kopecks / 100)
}

export function formatSpentAt(iso: string, today = new Date()): string {
  const [yearText, monthText, dayText] = iso.split('-')
  const year = Number(yearText)
  const month = Number(monthText)
  const day = Number(dayText)
  const date = new Date(year, month - 1, day)
  return new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: year === today.getFullYear() ? undefined : 'numeric',
  }).format(date)
}

export function periodCaption(period: Period, today = new Date()): string {
  if (period === 'week') {
    return 'за неделю'
  }
  if (period === 'month') {
    const month = new Intl.DateTimeFormat('ru-RU', { month: 'long' }).format(today)
    return `за ${month}`
  }
  if (period === 'year') {
    return `за ${today.getFullYear()} год`
  }
  return 'за всё время'
}

export function expenseCount(count: number): string {
  const mod10 = count % 10
  const mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) {
    return `${count} трата`
  }
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return `${count} траты`
  }
  return `${count} трат`
}
