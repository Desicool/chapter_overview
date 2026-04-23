import { test, expect } from '@playwright/test'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const FIXTURE_PDF = path.join(__dirname, '../../output.lin.pdf')

test.describe('PDF viewer', () => {
  let taskUrl: string

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage()
    await page.goto('/')

    const fileInput = page.locator('input[type="file"]')
    await fileInput.setInputFiles(FIXTURE_PDF)
    await page.getByRole('button', { name: /process pdf/i }).click()
    await page.waitForURL(/\/tasks\/[a-f0-9-]+$/, { timeout: 15_000 })
    taskUrl = page.url()

    // Wait for at least one chapter row before closing
    await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 360_000 })
    await page.close()
  })

  test('View link navigates to viewer and canvas renders without worker error', async ({ page }) => {
    const workerErrors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') workerErrors.push(msg.text())
    })
    page.on('pageerror', (err) => workerErrors.push(err.message))

    await page.goto(taskUrl)
    await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 360_000 })

    // Click the first "View →" link
    await page.locator('table tbody tr').first().getByText(/view/i).click()

    // Should navigate to viewer URL
    await page.waitForURL(/\/tasks\/[a-f0-9-]+\/view\/\d+/, { timeout: 10_000 })

    // Canvas must render with non-zero dimensions
    const canvas = page.locator('canvas')
    await expect(canvas).toBeVisible({ timeout: 30_000 })

    const width = await canvas.evaluate((el: HTMLCanvasElement) => el.width)
    const height = await canvas.evaluate((el: HTMLCanvasElement) => el.height)
    expect(width).toBeGreaterThan(0)
    expect(height).toBeGreaterThan(0)

    // No PDF worker errors
    const fatal = workerErrors.filter(
      (e) => e.includes('fake worker') || e.includes('Failed to fetch dynamically'),
    )
    expect(fatal).toHaveLength(0)
  })

  test('page navigation controls work', async ({ page }) => {
    // Navigate directly to page 1 of the viewer to ensure Prev is disabled
    await page.goto(`${taskUrl}/view/1`)
    await page.waitForURL(/\/view\/1$/, { timeout: 10_000 })
    await expect(page.locator('canvas')).toBeVisible({ timeout: 30_000 })

    // Prev button should be disabled on first page
    await expect(page.getByText('← Prev')).toBeDisabled()

    // Next button should be present
    await expect(page.getByText('Next →')).toBeVisible()
  })
})
