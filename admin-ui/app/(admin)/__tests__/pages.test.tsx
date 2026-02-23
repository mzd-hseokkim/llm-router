import { render, screen, act } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'
import LoginPage from '@/app/login/page'
import DashboardPage from '@/app/(admin)/page'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => '/',
}))

vi.mock('@/lib/api', () => ({
  usage: { topSpenders: vi.fn().mockResolvedValue([]) },
  keys: { list: vi.fn().mockResolvedValue([]) },
  providerKeys: { list: vi.fn().mockResolvedValue([]) },
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
