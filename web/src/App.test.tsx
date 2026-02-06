import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import App from './App'

test('renders aiPanel heading', () => {
  render(<App />)
  expect(screen.getByText('aiPanel')).toBeDefined()
})
