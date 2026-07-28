import { expect, test } from '@playwright/test';

test('the landing page works without JavaScript', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();

  await page.goto('/');
  await expect(page.locator('main h1')).toHaveCount(1);
  await expect(page.locator('[data-live-queue]')).toBeVisible();
  await expect(page.locator('[data-live-restart]')).toBeDisabled();
  await expect(page.locator('[data-live-toggle]')).toBeDisabled();

  await context.close();
});

test('the running illustration hands the resource to the next agent', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/#workcell');

  const restart = page.locator('[data-live-restart]');
  await expect(restart).toBeEnabled();
  await restart.click();
  await expect(page.locator('[data-main-status]')).toHaveText('RUNNING 1 / 3');
  await expect(page.locator('[data-main-b-status]')).toHaveText('RUNNING', { timeout: 3_000 });
  await expect(page.locator('[data-main-a-status]')).toHaveText('DONE');
  await expect(page.locator('[data-main-c-status]')).toHaveText('RUNNING', { timeout: 3_000 });
  await expect(page.locator('[data-main-b-status]')).toHaveText('DONE');
});

test('the landing page stays within a mobile viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  const widths = await page.evaluate(() => ({
    viewport: window.innerWidth,
    document: document.documentElement.scrollWidth,
  }));

  expect(widths.document).toBeLessThanOrEqual(widths.viewport);
});
