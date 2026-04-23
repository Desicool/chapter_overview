import { test, expect } from '@playwright/test'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const FIXTURE_PDF = path.join(__dirname, '../../output.lin.pdf')

test.describe('task page — chapter table and metrics', () => {
  let taskUrl: string

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage()
    await page.goto('/')

    const fileInput = page.locator('input[type="file"]')
    await fileInput.setInputFiles(FIXTURE_PDF)
    await page.getByRole('button', { name: /process pdf/i }).click()
    await page.waitForURL(/\/tasks\/[a-f0-9-]+$/, { timeout: 15_000 })
    taskUrl = page.url()
    await page.close()
  })

  test('chapter table shows Ch. N badges for every row after task completes', async ({ page }) => {
    await page.goto(taskUrl)

    // Wait for "Done" status pill — pipeline can take up to 6 minutes
    await expect(page.getByText('Done')).toBeVisible({ timeout: 360_000 })

    // At least one chapter row must appear
    const rows = page.locator('table tbody tr')
    await expect(rows.first()).toBeVisible()

    const count = await rows.count()
    expect(count).toBeGreaterThan(0)

    for (let i = 0; i < count; i++) {
      const badge = rows.nth(i).locator('td').first()
      await expect(badge).toHaveText(/^Ch\.\s*\d+$/)
    }
  })

  test('metrics panel shows Avg/page as a whole number', async ({ page }) => {
    await page.goto(taskUrl)

    // Wait for task done
    await expect(page.getByText('Done')).toBeVisible({ timeout: 360_000 })

    // Avg/page value should be a whole number (no dash, no decimal)
    const avgPageCell = page.getByText('Avg/page').locator('..').locator('span').last()
    await expect(avgPageCell).not.toHaveText('—')
    const text = await avgPageCell.innerText()
    expect(text.trim()).toMatch(/^[\d,]+$/)
  })

  test('metrics panel shows non-dash values for Input, Output, Max/call', async ({ page }) => {
    await page.goto(taskUrl)
    await expect(page.getByText('Done')).toBeVisible({ timeout: 360_000 })

    for (const label of ['Input', 'Output', 'Max/call']) {
      const cell = page.getByText(label, { exact: true }).locator('..').locator('span').last()
      await expect(cell).not.toHaveText('—')
    }
  })
})
