import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import AuditPage from '@/app/(admin)/audit/page'
import { auditLogs } from '@/lib/api'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/audit',
}))

vi.mock('@/lib/api', () => ({
  auditLogs: {
    list: vi.fn(),
  },
}))

function withQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
}

const mockEvent = {
  timestamp: '2026-02-01T10:00:00Z',
  event_type: 'CREATE',
  action: 'create_key',
  actor_type: 'admin',
  actor_email: 'admin@example.com',
  ip_address: '127.0.0.1',
  resource_type: 'key',
  resource_name: 'test-key',
  request_id: 'req-001',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('AuditPage', () => {
  it('TC-1: renders heading and empty state', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 0, page: 1, limit: 50, events: [] })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByText('Audit Logs')).toBeInTheDocument()
      expect(screen.getByText('No audit logs found.')).toBeInTheDocument()
    })
  })

  it('TC-2: renders event data in table', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 1, page: 1, limit: 50, events: [mockEvent] })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByText('admin@example.com')).toBeInTheDocument()
      expect(screen.getByText('create_key')).toBeInTheDocument()
    })
  })

  it('TC-2b: shows — for empty actor_email', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({
      total: 1, page: 1, limit: 50,
      events: [{ ...mockEvent, actor_email: '' }],
    })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByText('—')).toBeInTheDocument()
    })
  })

  it('TC-3: shows loading state', () => {
    vi.mocked(auditLogs.list).mockImplementation(() => new Promise(() => {}))
    render(withQueryClient(<AuditPage />))
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('TC-4: shows error state', async () => {
    vi.mocked(auditLogs.list).mockRejectedValue(new Error('Server error'))
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByText('Failed to load audit logs.')).toBeInTheDocument()
    })
  })

  it('TC-5: export buttons disabled when no events', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 0, page: 1, limit: 50, events: [] })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Export CSV' })).toBeDisabled()
      expect(screen.getByRole('button', { name: 'Export JSON' })).toBeDisabled()
    })
  })

  it('TC-6: export buttons enabled when events exist', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 1, page: 1, limit: 50, events: [mockEvent] })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Export CSV' })).not.toBeDisabled()
      expect(screen.getByRole('button', { name: 'Export JSON' })).not.toBeDisabled()
    })
  })

  it('TC-7: Apply triggers refetch with event_type filter', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 0, page: 1, limit: 50, events: [] })
    render(withQueryClient(<AuditPage />))
    await waitFor(() => expect(auditLogs.list).toHaveBeenCalledTimes(1))

    fireEvent.change(screen.getByLabelText('Event Type'), { target: { value: 'DELETE' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => {
      expect(auditLogs.list).toHaveBeenCalledWith(
        expect.objectContaining({ event_type: 'DELETE' })
      )
    })
  })

  it('TC-8: Apply button disabled when from > to', async () => {
    vi.mocked(auditLogs.list).mockResolvedValue({ total: 0, page: 1, limit: 50, events: [] })
    render(withQueryClient(<AuditPage />))

    fireEvent.change(screen.getByLabelText('Date From'), { target: { value: '2026-02-20' } })
    fireEvent.change(screen.getByLabelText('Date To'), { target: { value: '2026-02-10' } })

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  })
})
