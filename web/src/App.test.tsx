import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import App from './App'

afterEach(() => {
  vi.unstubAllGlobals()
})

test('renders login screen by default', async () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: false })
  vi.stubGlobal('fetch', fetchMock)
  render(<App />)
  expect(await screen.findByText('Sign in to continue.')).toBeDefined()
})
