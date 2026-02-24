import { render, screen, act, fireEvent, waitFor } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import KeysPage from '@/app/(admin)/keys/page'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/keys',
}))

const mockKeysApi = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue({ data: [], total: 0 }),
  create: vi.fn(),
  deactivate: vi.fn(),
  regenerate: vi.fn(),
}))

const mockCircuitBreakers = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue([]),
  reset: vi.fn().mockResolvedValue({ provider: 'openai', state: 'closed', message: 'ok' }),
}))

vi.mock('@/lib/api', () => ({
  keys: mockKeysApi,
  circuitBreakers: mockCircuitBreakers,
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

beforeEach(() => {
  vi.clearAllMocks()
  mockKeysApi.list.mockResolvedValue({ data: [], total: 0 })
  mockCircuitBreakers.list.mockResolvedValue([])
  mockCircuitBreakers.reset.mockResolvedValue({ provider: 'openai', state: 'closed', message: 'ok' })
})

describe('CircuitBreakerStateBadge', () => {
  it('CLOSED 상태 — emerald 색상 클래스, "CLOSED" 텍스트', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'closed', failure_count: 0 },
    ])
    render(withQueryClient(<KeysPage />))
    const badge = await screen.findByText('CLOSED')
    expect(badge.className).toContain('emerald')
  })

  it('OPEN 상태 — red 색상 클래스, "OPEN" 텍스트', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'open', failure_count: 3 },
    ])
    render(withQueryClient(<KeysPage />))
    const badge = await screen.findByText('OPEN')
    expect(badge.className).toContain('red')
  })

  it('HALF_OPEN 상태 — amber 색상 클래스, "HALF OPEN" 텍스트', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'half_open', failure_count: 1 },
    ])
    render(withQueryClient(<KeysPage />))
    const badge = await screen.findByText('HALF OPEN')
    expect(badge.className).toContain('amber')
  })

  it('알 수 없는 상태 — slate 기본 클래스 적용', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'unknown', failure_count: 0 },
    ])
    render(withQueryClient(<KeysPage />))
    const badge = await screen.findByText('UNKNOWN')
    expect(badge.className).toContain('slate')
  })
})

describe('Circuit Breaker 섹션 렌더링', () => {
  it('로딩 중 — "Loading…" 텍스트 표시', () => {
    mockCircuitBreakers.list.mockReturnValue(new Promise(() => {}))
    render(withQueryClient(<KeysPage />))
    expect(screen.getByText('Circuit Breakers')).toBeInTheDocument()
    expect(screen.getAllByText('Loading…').length).toBeGreaterThan(0)
  })

  it('빈 목록 — "No circuit breakers tracked" 표시', async () => {
    mockCircuitBreakers.list.mockResolvedValue([])
    render(withQueryClient(<KeysPage />))
    expect(await screen.findByText('No circuit breakers tracked')).toBeInTheDocument()
  })

  it('데이터 있음 — provider 이름, 상태 뱃지, failure_count 표시', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'open', failure_count: 5 },
      { provider: 'anthropic', state: 'closed', failure_count: 0 },
    ])
    render(withQueryClient(<KeysPage />))
    expect(await screen.findByText('openai')).toBeInTheDocument()
    expect(screen.getByText('anthropic')).toBeInTheDocument()
    expect(screen.getByText('OPEN')).toBeInTheDocument()
    expect(screen.getByText('CLOSED')).toBeInTheDocument()
    expect(screen.getByText('Failures: 5')).toBeInTheDocument()
    expect(screen.getByText('Failures: 0')).toBeInTheDocument()
  })

  it('Reset 버튼 활성화 — state="open"', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'open', failure_count: 3 },
    ])
    render(withQueryClient(<KeysPage />))
    const btn = await screen.findByRole('button', { name: 'Reset' })
    expect(btn).not.toBeDisabled()
  })

  it('Reset 버튼 비활성화 — state="closed"', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'closed', failure_count: 0 },
    ])
    render(withQueryClient(<KeysPage />))
    const btn = await screen.findByRole('button', { name: 'Reset' })
    expect(btn).toBeDisabled()
  })

  it('Reset 버튼 활성화 — state="half_open"', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'half_open', failure_count: 1 },
    ])
    render(withQueryClient(<KeysPage />))
    const btn = await screen.findByRole('button', { name: 'Reset' })
    expect(btn).not.toBeDisabled()
  })

  it('reset_time / last_failure 없을 때 "—" 표시', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'closed', failure_count: 0 },
    ])
    render(withQueryClient(<KeysPage />))
    await screen.findByText('CLOSED')
    expect(screen.getByText(/Last failure:.*—/)).toBeInTheDocument()
    expect(screen.getByText(/Reset time:.*—/)).toBeInTheDocument()
  })
})

describe('Circuit Breaker 인터랙션', () => {
  it('Reset 성공 — circuitBreakers.reset 호출됨', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'open', failure_count: 3 },
    ])
    render(withQueryClient(<KeysPage />))
    const btn = await screen.findByRole('button', { name: 'Reset' })
    await act(async () => {
      fireEvent.click(btn)
    })
    expect(mockCircuitBreakers.reset).toHaveBeenCalledWith('openai')
  })

  it('Reset 실패 — 에러 메시지 표시', async () => {
    mockCircuitBreakers.list.mockResolvedValue([
      { provider: 'openai', state: 'open', failure_count: 3 },
    ])
    mockCircuitBreakers.reset.mockRejectedValue(new Error('server error'))
    render(withQueryClient(<KeysPage />))
    const btn = await screen.findByRole('button', { name: 'Reset' })
    await act(async () => {
      fireEvent.click(btn)
    })
    await waitFor(() => {
      expect(screen.getByText('Reset failed. Try again.')).toBeInTheDocument()
    })
  })

  it('자동 갱신 설정 — circuitBreakers.list 호출 확인', async () => {
    mockCircuitBreakers.list.mockResolvedValue([])
    render(withQueryClient(<KeysPage />))
    await screen.findByText('No circuit breakers tracked')
    expect(mockCircuitBreakers.list).toHaveBeenCalledTimes(1)
  })
})
