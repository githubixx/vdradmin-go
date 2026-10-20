import { test, expect, themes, representatives, selectTheme, openPage, screenshot, assertWatchLayout } from './fixtures.js';

const sizes = {
  desktop: { width: 1440, height: 1000 },
  mobile: { width: 390, height: 844 },
};

test('theme inventory is preserved', async ({ request }) => {
  const response = await request.get('/__test/ready');
  expect(response.ok()).toBeTruthy();
  const metadata = await response.json();
  expect(metadata.themes.slice().sort()).toEqual(themes.slice().sort());
});

for (const theme of themes) {
  for (const [size, viewport] of Object.entries(sizes)) {
    test(`${theme} ${size} reference pages`, async ({ page, request }) => {
      await selectTheme(request, theme);
      await page.setViewportSize(viewport);
      await openPage(page, '/channels?day=2026-10-17');
      await expect(page.locator('.epg-item').first()).toBeVisible();
      await screenshot(page, `${theme}-${size}-channels`);
      await openPage(page, '/timers?day=2026-10-17');
      await expect(page.locator('.timer-item').first()).toBeVisible();
      await screenshot(page, `${theme}-${size}-timers`);
    });
  }
}

for (const theme of representatives) {
  for (const [size, viewport] of Object.entries(sizes)) {
    test(`${theme} ${size} extended pages`, async ({ page, request }) => {
      await selectTheme(request, theme);
      await page.setViewportSize(viewport);
      for (const [name, url] of [
        ['recordings', '/recordings'],
        ['configurations', '/configurations'],
        ['watch', '/watch?interval=0'],
      ]) {
        await openPage(page, url);
        if (name === 'watch') await assertWatchLayout(page);
        await screenshot(page, `${theme}-${size}-${name}`);
      }
      await openPage(page, '/channels?day=2026-10-17');
      const information = page.getByRole('link', { name: 'Information', exact: true }).first();
      const eventURL = await information.getAttribute('href');
      expect(eventURL).toBeTruthy();
      await openPage(page, eventURL);
      await screenshot(page, `${theme}-${size}-event`);
    });
  }
}

for (const colorScheme of ['light', 'dark']) {
  test(`system follows ${colorScheme} preference`, async ({ page, request }) => {
    await selectTheme(request, 'system');
    await page.emulateMedia({ colorScheme });
    await openPage(page, '/channels?day=2026-10-17');
    await expect(page.locator('html')).not.toHaveAttribute('data-theme');
    await screenshot(page, `system-${colorScheme}-channels`);
    const before = await page.locator('body').evaluate(body => getComputedStyle(body).backgroundColor);
    await page.emulateMedia({ colorScheme: colorScheme === 'light' ? 'dark' : 'light' });
    await expect.poll(() => page.locator('body').evaluate(body => getComputedStyle(body).backgroundColor))
      .not.toBe(before);
  });
}
