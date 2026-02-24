import { render, screen, waitFor, act } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import ReportsPage from '@/app/(admin)/reports/page'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/reports',
}))

const mockChargeback = vi.hoisted(() => ({
  period: '2026-02',
  generated_at: '2026-02-24T00:00:00Z',
  currency: 'USD',
  summary: { total_cost_usd: 5.1234, total_tokens: 10000, total_requests: 100 },
  by_team: [
    {
      team_id: 't1',
      team_name: 'Team Alpha',
      cost_usd: 3.0,
      markup_usd: 0.3,
      total_charged_usd: 3.3,
      tokens: 6000,
      requests: 60,
    },
  ],
}))

vi.mock('@/lib/api', () => ({
  reports: {
    chargeback: vi.fn().mockResolvedValue(mockChargeback),
    showback: vi.fn().mockResolvedValue(null),
  },
  teams: {
    list: vi.fn().mockResolvedValue([{ id: 't1', name: 'Team Alpha', created_at: '', updated_at: '' }]),
  },
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

describe('ReportsPage', () => {
  it('renders page heading', () => {
    render(withQueryClient(<ReportsPage />))
    expect(screen.getByText('Reports')).toBeInTheDocument()
    expect(screen.getByText('차지백 / 쇼백 비용 리포트')).toBeInTheDocument()
  })

  it('renders chargeback and showback tab buttons', () => {
    render(withQueryClient(<ReportsPage />))
    expect(screen.getByRole('button', { name: 'Chargeback' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Showback' })).toBeInTheDocument()
  })

  it('renders month input', () => {
    render(withQueryClient(<ReportsPage />))
    expect(screen.getByDisplayValue(/^\d{4}-\d{2}$/)).toBeInTheDocument()
  })

  it('renders chargeback summary stat cards after data loads', async () => {
    render(withQueryClient(<ReportsPage />))
    await waitFor(() => expect(screen.getByText('$5.1234')).toBeInTheDocument())
    expect(screen.getByText('10,000')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
  })

  it('renders team row in chargeback table', async () => {
    render(withQueryClient(<ReportsPage />))
    await waitFor(() => expect(screen.getByText('Team Alpha')).toBeInTheDocument())
    expect(screen.getByText('3.3000')).toBeInTheDocument()
  })

  it('renders CSV export button', async () => {
    render(withQueryClient(<ReportsPage />))
    await waitFor(() => expect(screen.getByRole('button', { name: 'CSV 내보내기' })).toBeInTheDocument())
  })

  it('renders showback team selector when showback tab is active', async () => {
    render(withQueryClient(<ReportsPage />))
    await act(async () => {
      screen.getByRole('button', { name: 'Showback' }).click()
    })
    await waitFor(() => expect(screen.getByText('— 팀을 선택하세요 —')).toBeInTheDocument())
    expect(screen.getByText('팀을 선택하면 쇼백 데이터가 표시됩니다.')).toBeInTheDocument()
  })
})
