import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import StatCard from '@/components/StatCard'

describe('StatCard', () => {
  it('renders title and string value', () => {
    render(<StatCard title="Total Requests" value="1,234" />)
    expect(screen.getByText('Total Requests')).toBeInTheDocument()
    expect(screen.getByText('1,234')).toBeInTheDocument()
  })

  it('renders numeric value', () => {
    render(<StatCard title="Active Keys" value={5} />)
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('renders sub text when provided', () => {
    render(<StatCard title="Active Keys" value={5} sub="of 10 total" />)
    expect(screen.getByText('of 10 total')).toBeInTheDocument()
  })

  it('does not render sub element when omitted', () => {
    render(<StatCard title="Monthly Cost" value="$1.23" />)
    expect(screen.queryByText(/of/)).not.toBeInTheDocument()
  })

  it('applies colorClass to value element', () => {
    render(<StatCard title="Test" value="42" colorClass="text-red-500" />)
    expect(screen.getByText('42')).toHaveClass('text-red-500')
  })

  it('uses default colorClass when not provided', () => {
    render(<StatCard title="Test" value="0" />)
    expect(screen.getByText('0')).toHaveClass('text-slate-900')
  })
})
