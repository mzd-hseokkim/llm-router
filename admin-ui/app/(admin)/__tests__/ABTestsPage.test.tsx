import { render, screen, act, fireEvent, waitFor } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import ABTestsPage from '@/app/(admin)/ab-tests/page'
import Sidebar from '@/components/Sidebar'
import { abTests, ABTest, ABTestTrafficSplit } from '@/lib/api'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/ab-tests',
}))

vi.mock('@/lib/api', () => ({
  abTests: {
    list:    vi.fn(),
    get:     vi.fn(),
    create:  vi.fn(),
    results: vi.fn(),
    start:   vi.fn(),
    pause:   vi.fn(),
    stop:    vi.fn(),
    promote: vi.fn(),
  },
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

const sampleTest: ABTest = {
  id: 'test-1',
  name: 'GPT vs Claude',
  status: 'running',
  traffic_split: [
    { variant: 'control',   weight: 50 } as ABTestTrafficSplit,
    { variant: 'treatment', weight: 50 } as ABTestTrafficSplit,
  ],
  target: { model: 'gpt-4o', sample_rate: 1.0 },
  min_samples: 1000,
  confidence_level: 0.95,
}

const sampleAnalysis = {
  test_id: 'test-1',
  status: 'running',
  results: {
    control:   { model: 'gpt-4o',            samples: 500, latency_p95_ms: 432.1, avg_cost_per_request: 0.0023, error_rate: 0.008 },
    treatment: { model: 'claude-3-5-sonnet', samples: 498, latency_p95_ms: 380.5, avg_cost_per_request: 0.0019, error_rate: 0.004 },
  },
  statistical_significance: {
    latency_p95_ms: { p_value: 0.023, significant: true, improvement_pct: -11.7 },
  },
  recommendation: 'treatment shows lower latency and cost',
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(abTests.list).mockResolvedValue([])
  vi.mocked(abTests.results).mockResolvedValue({} as Record<string, unknown>)
  vi.mocked(abTests.start).mockResolvedValue({ status: 'running' })
  vi.mocked(abTests.pause).mockResolvedValue({ status: 'paused' })
  vi.mocked(abTests.stop).mockResolvedValue({ status: 'stopped' })
  vi.mocked(abTests.promote).mockResolvedValue({ status: 'completed', winner: 'treatment' })
  vi.mocked(abTests.create).mockResolvedValue(sampleTest)
})

// ---------------------------------------------------------------------------
// ABTestsPage tests
// ---------------------------------------------------------------------------

describe('ABTestsPage', () => {

  it('TC-1: renders experiment list with name and status badge', async () => {
    vi.mocked(abTests.list).mockResolvedValue([sampleTest])
    render(withQueryClient(<ABTestsPage />))
    expect(await screen.findByText('GPT vs Claude')).toBeInTheDocument()
    expect(screen.getByText('running')).toBeInTheDocument()
  })

  it('TC-2: shows empty state when no experiments', async () => {
    render(withQueryClient(<ABTestsPage />))
    expect(await screen.findByText(/No A\/B tests/i)).toBeInTheDocument()
  })

  it('TC-3: shows loading indicator before data arrives', () => {
    vi.mocked(abTests.list).mockReturnValue(new Promise(() => {}))
    render(withQueryClient(<ABTestsPage />))
    expect(screen.getByText(/Loading/i)).toBeInTheDocument()
  })

  it('TC-4: opens CreateDialog on "+ New A/B Test" click', async () => {
    render(withQueryClient(<ABTestsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New A/B Test'))
    expect(screen.getByRole('heading', { name: /New A\/B Test/i })).toBeInTheDocument()
  })

  it('TC-5: closes CreateDialog on Cancel click', async () => {
    render(withQueryClient(<ABTestsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New A/B Test'))
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByRole('heading', { name: /New A\/B Test/i })).not.toBeInTheDocument()
  })

  it('TC-6: disables Create button when traffic weights do not sum to 100', async () => {
    render(withQueryClient(<ABTestsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New A/B Test'))

    // name 입력
    const nameInput = screen.getByPlaceholderText(/experiment name/i)
    fireEvent.change(nameInput, { target: { value: 'My Test' } })

    // weight를 불균등하게 설정
    const weightInputs = screen.getAllByRole('spinbutton')
    fireEvent.change(weightInputs[0], { target: { value: '60' } })

    // Submit 버튼이 disabled인지 확인
    const submitBtn = screen.getByRole('button', { name: /Create Experiment/i })
    expect(submitBtn).toBeDisabled()
    // 경고 메시지 확인
    expect(screen.getByText(/must sum to 100/i)).toBeInTheDocument()
  })

  it('TC-7: calls abTests.create and closes dialog on success', async () => {
    render(withQueryClient(<ABTestsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New A/B Test'))

    // 필수 필드 입력
    fireEvent.change(screen.getByPlaceholderText(/experiment name/i), { target: { value: 'Test' } })
    const modelInputs = screen.getAllByPlaceholderText(/e\.g\. gpt-4o/i)
    fireEvent.change(modelInputs[0], { target: { value: 'gpt-4o' } })
    fireEvent.change(modelInputs[1], { target: { value: 'claude-3-5-sonnet' } })

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Create Experiment/i }))
    })

    expect(abTests.create).toHaveBeenCalled()
    expect(screen.queryByRole('heading', { name: /New A\/B Test/i })).not.toBeInTheDocument()
  })

  it('TC-8: calls abTests.start when Start button is clicked', async () => {
    const draftTest = { ...sampleTest, status: 'draft' }
    vi.mocked(abTests.list).mockResolvedValue([draftTest])
    render(withQueryClient(<ABTestsPage />))
    await screen.findByText('GPT vs Claude')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^Start$/i }))
    })
    expect(abTests.start).toHaveBeenCalledWith('test-1')
  })

  it('TC-9: opens ResultsPanel with variant data on Results button click', async () => {
    vi.mocked(abTests.list).mockResolvedValue([sampleTest])
    vi.mocked(abTests.results).mockResolvedValue(sampleAnalysis as unknown as Record<string, unknown>)
    render(withQueryClient(<ABTestsPage />))
    await screen.findByText('GPT vs Claude')
    fireEvent.click(screen.getByRole('button', { name: /Results/i }))
    expect(await screen.findByText('control')).toBeInTheDocument()
    expect(screen.getByText('treatment')).toBeInTheDocument()
  })

  it('TC-10: opens PromoteDialog and calls abTests.promote on confirm', async () => {
    vi.mocked(abTests.list).mockResolvedValue([sampleTest])
    render(withQueryClient(<ABTestsPage />))
    await screen.findByText('GPT vs Claude')
    fireEvent.click(screen.getByRole('button', { name: /Promote/i }))

    // PromoteDialog가 열렸는지 확인
    expect(screen.getByRole('heading', { name: /Promote Winner/i })).toBeInTheDocument()

    // variant 선택
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'treatment' } })

    // 다이얼로그 내 Promote 버튼 (테이블 버튼과 구분 — getAllByRole로 마지막 항목 선택)
    const promoteButtons = screen.getAllByRole('button', { name: /^Promote$/i })
    await act(async () => {
      fireEvent.click(promoteButtons[promoteButtons.length - 1])
    })

    expect(abTests.promote).toHaveBeenCalledWith('test-1', 'treatment')
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: /Promote Winner/i })).not.toBeInTheDocument()
    })
  })

})

// ---------------------------------------------------------------------------
// Sidebar A/B Tests link
// ---------------------------------------------------------------------------

describe('Sidebar', () => {
  it('TC-11: has A/B Tests link with href=/ab-tests', () => {
    render(<Sidebar />)
    const link = screen.getByRole('link', { name: 'A/B Tests' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/ab-tests')
  })
})
