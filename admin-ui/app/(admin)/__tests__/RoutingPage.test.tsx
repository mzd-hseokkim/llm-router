import { render, screen, act, fireEvent, waitFor } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import RoutingPage from '@/app/(admin)/routing/page'
import { routingRules, RoutingRule, RoutingRuleTarget } from '@/lib/api'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/routing',
}))

vi.mock('@/lib/api', () => ({
  routingRules: {
    list:    vi.fn(),
    create:  vi.fn(),
    update:  vi.fn(),
    delete:  vi.fn(),
    reload:  vi.fn(),
    dryRun:  vi.fn(),
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

const sampleRule: RoutingRule = {
  id: 'rule-1',
  name: 'GPT-4 Direct',
  priority: 10,
  enabled: true,
  strategy: 'direct',
  match: { model: 'gpt-4o' },
  targets: [{ provider: 'openai', model: 'gpt-4o' }],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const weightedRule: RoutingRule = {
  id: 'rule-2',
  name: 'Weighted LB',
  priority: 50,
  enabled: false,
  strategy: 'weighted',
  match: {},
  targets: [
    { provider: 'openai', model: 'gpt-4o', weight: 60 },
    { provider: 'anthropic', model: 'claude-3-5-sonnet', weight: 40 },
  ],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(routingRules.list).mockResolvedValue([])
  vi.mocked(routingRules.create).mockResolvedValue(sampleRule)
  vi.mocked(routingRules.update).mockResolvedValue(sampleRule)
  vi.mocked(routingRules.delete).mockResolvedValue(new Response(null, { status: 204 }))
  vi.mocked(routingRules.reload).mockResolvedValue({ status: 'ok' })
  vi.mocked(routingRules.dryRun).mockResolvedValue({
    matched_rule: null,
    strategy: 'direct',
    targets: [] as RoutingRuleTarget[],
  })
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('RoutingPage', () => {

  it('TC-1: renders rule list with name, priority, and strategy badge', async () => {
    vi.mocked(routingRules.list).mockResolvedValue([sampleRule])
    render(withQueryClient(<RoutingPage />))
    expect(await screen.findByText('GPT-4 Direct')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('direct')).toBeInTheDocument()
  })

  it('TC-2: shows empty state when no rules', async () => {
    render(withQueryClient(<RoutingPage />))
    expect(await screen.findByText(/No routing rules/i)).toBeInTheDocument()
  })

  it('TC-3: shows loading indicator before data arrives', () => {
    vi.mocked(routingRules.list).mockReturnValue(new Promise(() => {}))
    render(withQueryClient(<RoutingPage />))
    expect(screen.getByText(/Loading/i)).toBeInTheDocument()
  })

  it('TC-4: opens Add Rule dialog on "+ Add Rule" click', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ Add Rule'))
    expect(screen.getByRole('heading', { name: /Add Rule/i })).toBeInTheDocument()
  })

  it('TC-5: Edit button opens dialog with existing values filled in', async () => {
    vi.mocked(routingRules.list).mockResolvedValue([sampleRule])
    render(withQueryClient(<RoutingPage />))
    await screen.findByText('GPT-4 Direct')
    fireEvent.click(screen.getByRole('button', { name: /^Edit$/i }))
    const nameInput = screen.getByRole('textbox', { name: /Name/i }) as HTMLInputElement
    expect(nameInput.value).toBe('GPT-4 Direct')
  })

  it('TC-6: Delete button → confirm dialog → calls routingRules.delete', async () => {
    vi.mocked(routingRules.list).mockResolvedValue([sampleRule])
    render(withQueryClient(<RoutingPage />))
    await screen.findByText('GPT-4 Direct')
    fireEvent.click(screen.getByRole('button', { name: /^Delete$/i }))
    // Confirm dialog should be visible
    expect(screen.getByRole('heading', { name: /Delete Rule/i })).toBeInTheDocument()
    // Click confirm
    const deleteButtons = screen.getAllByRole('button', { name: /^Delete$/i })
    await act(async () => {
      fireEvent.click(deleteButtons[deleteButtons.length - 1])
    })
    expect(routingRules.delete).toHaveBeenCalledWith('rule-1')
  })

  it('TC-7: Cancel in DeleteConfirmDialog does not call delete', async () => {
    vi.mocked(routingRules.list).mockResolvedValue([sampleRule])
    render(withQueryClient(<RoutingPage />))
    await screen.findByText('GPT-4 Direct')
    fireEvent.click(screen.getByRole('button', { name: /^Delete$/i }))
    fireEvent.click(screen.getByRole('button', { name: /^Cancel$/i }))
    expect(routingRules.delete).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: /Delete Rule/i })).not.toBeInTheDocument()
    })
  })

  it('TC-8: Add Rule → fill name and target → Save calls routingRules.create', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ Add Rule'))

    // Fill name
    fireEvent.change(screen.getByPlaceholderText('Rule name'), {
      target: { value: 'New Rule' },
    })
    // Fill target provider and model
    const providerInputs = screen.getAllByPlaceholderText('provider')
    const modelInputs = screen.getAllByPlaceholderText('model')
    fireEvent.change(providerInputs[0], { target: { value: 'openai' } })
    fireEvent.change(modelInputs[0], { target: { value: 'gpt-4o' } })

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^Save$/i }))
    })

    expect(routingRules.create).toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: /Add Rule/i })).not.toBeInTheDocument()
    })
  })

  it('TC-9: weighted strategy → weight input fields shown', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ Add Rule'))

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'weighted' } })

    // weight placeholder input should be visible
    await waitFor(() => {
      expect(screen.getAllByPlaceholderText('weight').length).toBeGreaterThan(0)
    })
  })

  it('TC-10: weighted strategy with weight sum != 100 → Save button disabled', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ Add Rule'))

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'weighted' } })
    fireEvent.change(screen.getByPlaceholderText('Rule name'), {
      target: { value: 'My Rule' },
    })

    // Two targets exist after switching to weighted; set weights to 60+60
    const weightInputs = screen.getAllByPlaceholderText('weight')
    fireEvent.change(weightInputs[0], { target: { value: '60' } })
    fireEvent.change(weightInputs[1], { target: { value: '60' } })

    const saveBtn = screen.getByRole('button', { name: /^Save$/i })
    expect(saveBtn).toBeDisabled()
    expect(screen.getByText(/must sum to 100/i)).toBeInTheDocument()
  })

  it('TC-11: DryRun success — shows matched rule name and strategy', async () => {
    vi.mocked(routingRules.dryRun).mockResolvedValue({
      matched_rule: 'GPT-4 Direct',
      strategy: 'direct',
      targets: [{ provider: 'openai', model: 'gpt-4o' }],
    })
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})

    fireEvent.change(screen.getByPlaceholderText('e.g. gpt-4o'), {
      target: { value: 'gpt-4o' },
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^Test$/i }))
    })

    expect(await screen.findByText('GPT-4 Direct')).toBeInTheDocument()
    // strategy badge in dryrun result
    const directBadges = screen.getAllByText('direct')
    expect(directBadges.length).toBeGreaterThan(0)
  })

  it('TC-12: DryRun no match — shows "No matching rule" message', async () => {
    vi.mocked(routingRules.dryRun).mockResolvedValue({
      matched_rule: null,
      strategy: 'direct',
      targets: [],
    })
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})

    fireEvent.change(screen.getByPlaceholderText('e.g. gpt-4o'), {
      target: { value: 'unknown-model' },
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^Test$/i }))
    })

    expect(await screen.findByText(/No matching rule/i)).toBeInTheDocument()
  })

  it('TC-13: Reload button calls routingRules.reload', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^Reload$/i }))
    })
    expect(routingRules.reload).toHaveBeenCalled()
  })

  it('TC-14: direct strategy → Add Target button is disabled', async () => {
    render(withQueryClient(<RoutingPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ Add Rule'))

    // strategy defaults to "direct"
    const addTargetBtn = screen.getByRole('button', { name: /\+ Add Target/i })
    expect(addTargetBtn).toBeDisabled()
  })

})
