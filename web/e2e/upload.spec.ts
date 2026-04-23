import { test, expect } from '@playwright/test'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const FIXTURE_PDF = path.join(__dirname, '../../output.lin.pdf')

test('upload page renders with drag-drop zone', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('Reveal the Structure')).toBeVisible()
  await expect(page.locator('input[type="file"]')).toBeAttached()
})

test('uploading a PDF redirects to task page and shows progress', async ({ page }) => {
  await page.goto('/')

  const fileInput = page.locator('input[type="file"]')
  await fileInput.setInputFiles(FIXTURE_PDF)

  // Click submit after selecting file
  await page.getByRole('button', { name: /process pdf/i }).click()

  // Should navigate to /tasks/:id
  await page.waitForURL(/\/tasks\/[a-f0-9-]+$/, { timeout: 15_000 })

  // Thin amber progress bar container should be present
  await expect(page.locator('.h-1.bg-surface').first()).toBeVisible()
})
