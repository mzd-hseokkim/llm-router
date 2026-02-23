import { render, screen, act, waitFor } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import LoginPage from '@/app/login/page'
import DashboardPage from '@/app/(admin)/page'
import AlertsPage from '@/app/(admin)/alerts/page'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/',
}))

const mockConfig = vi.hoisted(() => ({
  channels: { slack: { webhook_url: '', enabled: false }, email: { addresses: [], enabled: false }, webhook: { url: '', enabled: false } },
  conditions: { budget_threshold_pct: 80, error_rate_threshold: 5, latency_threshold_ms: 2000 },
  enabled: true,
}))

vi.mock('@/lib/api', () => ({
  usage: { topSpenders: vi.fn().mockResolvedValue([]) },
  keys: { list: vi.fn().mockResolvedValue([]) },
  providerKeys: { list: vi.fn().mockResolvedValue([]) },
  alerts: {
    getConfig: vi.fn().mockResolvedValue(mockConfig),
    updateConfig: vi.fn().mockResolvedValue(mockConfig),
    test: vi.fn().mockResolvedValue({ status: 'ok' }),
    history: vi.fn().mockResolvedValue([]),
  },
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

describe('LoginPage', () => {
  it('renders admin key form', async () => {
    render(<LoginPage />)
    await act(async () => {})
    expect(screen.getByText('LLM Router Admin')).toBeInTheDocument()
    expect(screen.getByLabelText('Admin Key')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('renders password input', async () => {
    render(<LoginPage />)
    await act(async () => {})
    const input = screen.getByLabelText('Admin Key')
    expect(input).toHaveAttribute('type', 'password')
  })
})

describe('AlertsPage', () => {
  it('renders page heading', async () => {
    render(withQueryClient(<AlertsPage />))
    await waitFor(() => expect(screen.getByText('알림 설정')).toBeInTheDocument())
  })

  it('renders channel and condition section cards', async () => {
    render(withQueryClient(<AlertsPage />))
    await waitFor(() => expect(screen.getByText('채널 설정')).toBeInTheDocument())
    expect(screen.getByText('알림 조건')).toBeInTheDocument()
    expect(screen.getByText('최근 발송 히스토리')).toBeInTheDocument()
  })

  it('renders save button', async () => {
    render(withQueryClient(<AlertsPage />))
    await waitFor(() => expect(screen.getByRole('button', { name: '저장' })).toBeInTheDocument())
  })

  it('renders empty history message when no history', async () => {
    render(withQueryClient(<AlertsPage />))
    await waitFor(() => expect(screen.getByText('발송 기록이 없습니다.')).toBeInTheDocument())
  })
})

describe('DashboardPage', () => {
  it('renders dashboard heading', () => {
    render(withQueryClient(<DashboardPage />))
    expect(screen.getByText('Dashboard')).toBeInTheDocument()
  })

  it('renders all stat card titles', () => {
    render(withQueryClient(<DashboardPage />))
    expect(screen.getByText('Monthly Cost')).toBeInTheDocument()
    expect(screen.getByText('Total Requests')).toBeInTheDocument()
    expect(screen.getByText('Active Keys')).toBeInTheDocument()
    expect(screen.getByText('Active Provider Keys')).toBeInTheDocument()
  })

  it('shows empty state when no data', () => {
    render(withQueryClient(<DashboardPage />))
    expect(screen.getByText('No data')).toBeInTheDocument()
    expect(screen.getByText('No provider keys')).toBeInTheDocument()
  })
})
