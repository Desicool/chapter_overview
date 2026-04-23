import { test, expect } from '@playwright/test'

test('navbar links navigate without blank screens or 404', async ({ page }) => {
  await page.goto('/')
  await expect(page).not.toHaveURL(/404/)
  await expect(page.locator('body')).not.toBeEmpty()

  // Tasks nav link
  await page.getByRole('link', { name: /tasks/i }).first().click()
  await page.waitForURL(/\/tasks/)
  await expect(page.locator('body')).not.toBeEmpty()

  // Stats nav link
  await page.getByRole('link', { name: /stats/i }).first().click()
  await page.waitForURL(/\/stats/)
  await expect(page.locator('body')).not.toBeEmpty()

  // Upload nav link
  await page.getByRole('link', { name: /upload/i }).first().click()
  await page.waitForURL(/\/$|\/upload/)
  await expect(page.locator('body')).not.toBeEmpty()
})

test('task list page renders without blank screen', async ({ page }) => {
  await page.goto('/tasks')
  // Should show either a list of tasks or an empty state — not a blank page
  const bodyText = await page.locator('body').innerText()
  expect(bodyText.trim().length).toBeGreaterThan(0)
})

test('stats page renders two tabs', async ({ page }) => {
  await page.goto('/stats')
  await expect(page.getByText(/page view/i)).toBeVisible({ timeout: 10_000 })
  await expect(page.getByText(/task view/i)).toBeVisible({ timeout: 10_000 })
})
