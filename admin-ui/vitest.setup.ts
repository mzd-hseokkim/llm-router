import '@testing-library/jest-dom'
import { vi } from 'vitest'

// ResizeObserver is not available in jsdom; required by recharts ResponsiveContainer
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}))

// Default fetch mock — individual tests can override via vi.fn().mockResolvedValueOnce(...)
global.fetch = vi.fn().mockResolvedValue({
  ok: true,
  json: async () => ({}),
})
