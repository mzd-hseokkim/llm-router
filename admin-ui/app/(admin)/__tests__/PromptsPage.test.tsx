import { render, screen, act, fireEvent } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import PromptsPage from '@/app/(admin)/prompts/page'
import Sidebar from '@/components/Sidebar'
import { prompts } from '@/lib/api'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/prompts',
}))

vi.mock('@/lib/api', () => ({
  prompts: {
    list: vi.fn(),
    create: vi.fn(),
    versions: {
      list: vi.fn(),
      create: vi.fn(),
    },
    rollback: vi.fn(),
    diff: vi.fn(),
  },
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

const samplePrompt = {
  id: '1',
  slug: 'welcome',
  name: 'Welcome',
  template: 'Hello {{name}}',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const sampleVersion = {
  id: 'v1',
  version: '1.0.0',
  template: 'Hello {{name}}',
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(prompts.list).mockResolvedValue({ data: [] })
  vi.mocked(prompts.versions.list).mockResolvedValue([])
})

describe('PromptsPage', () => {
  it('TC-1: renders prompt list', async () => {
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    render(withQueryClient(<PromptsPage />))
    // findByText retries until element appears (handles async React Query)
    expect(await screen.findByText('Welcome')).toBeInTheDocument()
    expect(screen.getByText('welcome')).toBeInTheDocument()
  })

  it('TC-1: shows empty state when no prompts', async () => {
    render(withQueryClient(<PromptsPage />))
    expect(await screen.findByText(/No prompts/i)).toBeInTheDocument()
  })

  it('TC-2: shows loading state before data arrives', () => {
    vi.mocked(prompts.list).mockReturnValue(new Promise(() => {}))
    render(withQueryClient(<PromptsPage />))
    expect(screen.getByText(/Loading/i)).toBeInTheDocument()
  })

  it('TC-3: opens CreatePromptDialog on "+ New Prompt" click', async () => {
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))
    expect(screen.getByRole('heading', { name: 'New Prompt' })).toBeInTheDocument()
  })

  it('TC-3: closes CreatePromptDialog on Cancel', async () => {
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByRole('heading', { name: 'New Prompt' })).not.toBeInTheDocument()
  })

  it('TC-4: calls prompts.create and closes dialog on success', async () => {
    vi.mocked(prompts.create).mockResolvedValue({ prompt: samplePrompt, version: sampleVersion })
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))

    fireEvent.change(screen.getByPlaceholderText('my-prompt'), { target: { value: 'test' } })
    fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Test' } })
    fireEvent.change(screen.getByPlaceholderText(/variables/i), { target: { value: 'Hello' } })

    await act(async () => {
      fireEvent.click(screen.getByText('Create'))
    })

    expect(prompts.create).toHaveBeenCalledWith({ slug: 'test', name: 'Test', template: 'Hello' })
    expect(screen.queryByRole('heading', { name: 'New Prompt' })).not.toBeInTheDocument()
  })

  it('TC-5: shows error message and keeps dialog open on create failure', async () => {
    vi.mocked(prompts.create).mockRejectedValue(new Error('slug already exists'))
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))

    fireEvent.change(screen.getByPlaceholderText('my-prompt'), { target: { value: 'test' } })
    fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Test' } })
    fireEvent.change(screen.getByPlaceholderText(/variables/i), { target: { value: 'Hello' } })

    await act(async () => {
      fireEvent.click(screen.getByText('Create'))
    })

    expect(await screen.findByText('slug already exists')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'New Prompt' })).toBeInTheDocument()
  })

  it('TC-6: VariableEditor highlights {{variable}} patterns', async () => {
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))
    const textarea = screen.getByPlaceholderText(/variables/i)
    fireEvent.change(textarea, {
      target: { value: 'Hello {{name}}, today is {{date}}' },
    })
    const marks = document.querySelectorAll('mark')
    expect(marks.length).toBe(2)
  })

  it('TC-7: VariableEditor shows red warning for empty {{}} variable', async () => {
    render(withQueryClient(<PromptsPage />))
    await act(async () => {})
    fireEvent.click(screen.getByText('+ New Prompt'))
    const textarea = screen.getByPlaceholderText(/variables/i)
    fireEvent.change(textarea, { target: { value: 'Hello {{}}' } })
    const marks = document.querySelectorAll('mark')
    expect(marks.length).toBeGreaterThan(0)
    expect(marks[0].className).toContain('bg-red-200')
  })

  it('TC-8: shows version history panel with version list', async () => {
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([sampleVersion])
    render(withQueryClient(<PromptsPage />))
    // Wait for prompt row to appear
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    // Wait for version list to load
    expect(await screen.findByText('1.0.0')).toBeInTheDocument()
    expect(screen.getByText(/Active/i)).toBeInTheDocument()
  })

  it('TC-9: closes version panel on Close button click', async () => {
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([sampleVersion])
    render(withQueryClient(<PromptsPage />))
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    await screen.findByText('1.0.0')
    fireEvent.click(screen.getByText('✕ Close'))
    expect(screen.queryByText('1.0.0')).not.toBeInTheDocument()
  })

  it('TC-10: Rollback button is disabled for active version', async () => {
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([sampleVersion])
    render(withQueryClient(<PromptsPage />))
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    const rollbackBtn = await screen.findByText('Rollback')
    expect(rollbackBtn).toBeDisabled()
  })

  it('TC-11: Rollback calls prompts.rollback with correct args', async () => {
    const inactiveVersion = {
      ...sampleVersion,
      id: 'v2',
      version: '0.9.0',
      is_active: false,
    }
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([
      sampleVersion,
      inactiveVersion,
    ])
    vi.mocked(prompts.rollback).mockResolvedValue({ message: 'ok' })
    render(withQueryClient(<PromptsPage />))
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    // Wait for version panel to load (multiple '1.0.0' expected: table cell + select options)
    await screen.findAllByText('1.0.0')
    // Find the enabled Rollback button (for the inactive version)
    const rollbackBtns = screen.getAllByText('Rollback')
    const enabledBtn = rollbackBtns.find((btn) => !btn.hasAttribute('disabled'))
    expect(enabledBtn).toBeTruthy()
    await act(async () => {
      fireEvent.click(enabledBtn!)
    })
    expect(prompts.rollback).toHaveBeenCalledWith('welcome', '0.9.0')
  })

  it('TC-12: Compare button is disabled when no versions selected', async () => {
    const v2 = {
      ...sampleVersion,
      id: 'v2',
      version: '2.0.0',
      is_active: false,
    }
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([sampleVersion, v2])
    render(withQueryClient(<PromptsPage />))
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    const compareBtn = await screen.findByText('Compare')
    expect(compareBtn).toBeDisabled()
  })

  it('TC-13: DiffView renders added/removed lines', async () => {
    const v2 = {
      ...sampleVersion,
      id: 'v2',
      version: '2.0.0',
      is_active: false,
    }
    vi.mocked(prompts.list).mockResolvedValue({ data: [samplePrompt] })
    vi.mocked(prompts.versions.list).mockResolvedValue([sampleVersion, v2])
    vi.mocked(prompts.diff).mockResolvedValue({
      from: { version: '1.0.0', template: 'line1\nline2' },
      to: { version: '2.0.0', template: 'line1\nline3' },
    })
    render(withQueryClient(<PromptsPage />))
    await screen.findByText('Welcome')
    fireEvent.click(screen.getByText('Versions'))
    await screen.findByText('Compare')
    const selects = screen.getAllByRole('combobox')
    fireEvent.change(selects[0], { target: { value: '1.0.0' } })
    fireEvent.change(selects[1], { target: { value: '2.0.0' } })
    await act(async () => {
      fireEvent.click(screen.getByText('Compare'))
    })
    expect(await screen.findByText('line1')).toBeInTheDocument()
    expect(screen.getByText('line2')).toBeInTheDocument()
    expect(screen.getByText('line3')).toBeInTheDocument()
  })
})

describe('Sidebar', () => {
  it('TC-14: has Prompts link with href=/prompts', () => {
    render(<Sidebar />)
    const link = screen.getByRole('link', { name: 'Prompts' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/prompts')
  })
})
