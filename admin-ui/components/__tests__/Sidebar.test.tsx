import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import Sidebar from '@/components/Sidebar'

const mockPush = vi.hoisted(() => vi.fn())

vi.mock('next/navigation', () => ({
  usePathname: () => '/',
  useRouter: () => ({ push: mockPush, replace: vi.fn(), prefetch: vi.fn() }),
}))

beforeEach(() => {
  vi.clearAllMocks()
  ;(global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
    ok: true,
    json: async () => ({}),
  })
})

describe('Sidebar', () => {
  it('renders all navigation links', () => {
    render(<Sidebar />)
    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.getByText('Playground')).toBeInTheDocument()
    expect(screen.getByText('Virtual Keys')).toBeInTheDocument()
    expect(screen.getByText('Providers')).toBeInTheDocument()
    expect(screen.getByText('Organizations')).toBeInTheDocument()
    expect(screen.getByText('Guardrails')).toBeInTheDocument()
    expect(screen.getByText('Usage')).toBeInTheDocument()
    expect(screen.getByText('Logs')).toBeInTheDocument()
    expect(screen.getByText('Audit Logs')).toBeInTheDocument()
  })

  it('renders correct href for nav links', () => {
    render(<Sidebar />)
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/')
    expect(screen.getByRole('link', { name: 'Virtual Keys' })).toHaveAttribute('href', '/keys')
    expect(screen.getByRole('link', { name: 'Providers' })).toHaveAttribute('href', '/providers')
    expect(screen.getByRole('link', { name: 'Organizations' })).toHaveAttribute('href', '/orgs')
    expect(screen.getByRole('link', { name: 'Audit Logs' })).toHaveAttribute('href', '/audit')
  })

  it('renders Sign out button', () => {
    render(<Sidebar />)
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument()
  })

  it('calls logout API and redirects on sign out click', async () => {
    const user = userEvent.setup()
    render(<Sidebar />)
    await user.click(screen.getByRole('button', { name: 'Sign out' }))
    expect(global.fetch).toHaveBeenCalledWith('/api/auth/logout', { method: 'POST' })
    expect(mockPush).toHaveBeenCalledWith('/login')
  })
})
