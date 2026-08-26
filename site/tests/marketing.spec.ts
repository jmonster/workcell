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
  for (const route of ['/', '/stringproof/', '/playbook/', '/knowledge/']) {
    await page.goto(route);

    const widths = await page.evaluate(() => ({
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
    }));

    expect(widths.document).toBeLessThanOrEqual(widths.viewport);
  }
});

test('the desktop navigation does not wrap before yielding to mobile', async ({ page }) => {
  await page.setViewportSize({ width: 1000, height: 700 });
  await page.goto('/playbook/');

  const linkHeights = await page.locator('.desktop-navigation a').evaluateAll((links) =>
    links.map((link) => link.getBoundingClientRect().height),
  );
  expect(new Set(linkHeights)).toEqual(new Set([36]));

  await page.setViewportSize({ width: 800, height: 700 });
  await expect(page.locator('.desktop-navigation')).toBeHidden();
  await expect(page.locator('.mobile-navigation')).toBeVisible();
});

test('the header links to the studio homepage and author profile', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 700 });
  await page.goto('/');

  const desktopIdentity = page.locator('.identity-shortcuts');
  await expect(desktopIdentity).toHaveText(/@work\s*\|\s*@jmonster/);
  await expect(desktopIdentity.getByRole('link', { name: 'Wabi Sabi Ware' })).toHaveAttribute(
    'href',
    'https://wabisabiwar.es/',
  );
  await expect(desktopIdentity.getByRole('link', { name: '@jmonster on GitHub' })).toHaveAttribute(
    'href',
    'https://github.com/jmonster',
  );

  await page.setViewportSize({ width: 390, height: 700 });
  await page.locator('.mobile-navigation summary').click();
  const mobileIdentity = page.locator('.mobile-identity-shortcuts');
  await expect(mobileIdentity).toHaveText(/@work\s*\|\s*@jmonster/);
});

test('the header opens Knowledge without advertising access', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('.desktop-navigation a')).toHaveText([
    'Playbook',
    'Stringproof',
    'Knowledge',
    'Workcell',
  ]);
  await expect(page.getByRole('link', { name: 'Workcell', exact: true }).first()).toHaveClass(
    /is-primary/,
  );
  await expect(page.getByRole('link', { name: 'Knowledge', exact: true }).first()).toHaveClass(
    /is-secondary/,
  );
  await page.getByRole('link', { name: 'Knowledge' }).first().click();

  await expect(page).toHaveURL(/\/knowledge\/$/);
  await expect(page.locator('main h1')).toHaveText('Knowledge');
  await expect(page.getByRole('link', { name: 'Knowledge', exact: true }).first()).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.getByRole('link', { name: 'View on GitHub' })).toHaveAttribute(
    'href',
    'https://github.com/jmonster/workcell/tree/main/knowledge',
  );
  await expect(page.locator('head link[rel="alternate"][type="text/plain"]')).toHaveCount(0);
  await expect(page.getByRole('contentinfo')).toHaveCount(0);
});

test('the header opens Playbook and links to its source folder', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('link', { name: 'Playbook' }).first().click();

  await expect(page).toHaveURL(/\/playbook\/$/);
  await expect(page.locator('main h1')).toHaveText('Playbook');
  await expect(page.getByRole('link', { name: 'Playbook', exact: true }).first()).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.getByRole('link', { name: 'View on GitHub' })).toHaveAttribute(
    'href',
    'https://github.com/jmonster/workcell/tree/main/playbook',
  );
});

test('the header opens Stringproof and links to its source folder', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('link', { name: 'Workcell', exact: true }).first()).toHaveAttribute(
    'aria-current',
    'page',
  );
  await page.getByRole('link', { name: 'Stringproof' }).first().click();

  await expect(page).toHaveURL(/\/stringproof\/$/);
  await expect(page.locator('main h1')).toHaveText('Stringproof');
  await expect(page.getByRole('link', { name: 'Stringproof', exact: true }).first()).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.getByRole('link', { name: 'View on GitHub' })).toHaveAttribute(
    'href',
    'https://github.com/jmonster/workcell/tree/main/stringproof',
  );
  await expect(page.getByRole('link', { name: 'Source', exact: true })).toHaveCount(0);
});

test('each product exposes its own agent instructions', async ({ page }) => {
  const products = [
    {
      route: '/',
      instructions: '/llms.txt',
      title: 'Workcell instructions for language models',
    },
    {
      route: '/stringproof/',
      instructions: '/stringproof/llms.txt',
      title: 'Stringproof instructions for language models',
    },
    {
      route: '/playbook/',
      instructions: '/playbook/llms.txt',
      title: 'Playbook instructions for language models',
    },
  ];

  for (const product of products) {
    await page.goto(product.route);

    await expect(page.getByRole('contentinfo').getByRole('link', { name: 'Robots' })).toHaveAttribute(
      'href',
      product.instructions,
    );
    await expect(page.locator('head link[rel="alternate"][type="text/plain"]')).toHaveAttribute(
      'href',
      `https://workcell-137.pages.dev${product.instructions}`,
    );
    await expect(page.locator('head link[rel="alternate"][type="text/plain"]')).toHaveAttribute(
      'title',
      product.title,
    );
  }
});
