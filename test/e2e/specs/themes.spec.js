import { test, expect, themes, representatives, selectTheme, openPage, screenshot, assertWatchLayout, guideURL, prepareGuide, assertGuideLayout } from './fixtures.js';

const sizes = {
  desktop: { width: 1440, height: 1000 },
  mobile: { width: 390, height: 844 },
};

for (const theme of representatives) {
  for (const [size, viewport] of Object.entries(sizes)) {
    test(`${theme} ${size} TV Guide reference`, async ({ page, request }) => {
      await selectTheme(request, theme);
      await prepareGuide(request);
      await page.setViewportSize(viewport);
      await openPage(page, guideURL);
      await assertGuideLayout(page);
      await screenshot(page, `${theme}-${size}-playing-guide`);
      const trigger = page.locator('#playing-view').locator('..').getByRole('combobox');
      await trigger.click();
      await expect(page.getByRole('option', { name: 'TV Guide', exact: true })).toBeVisible();
      await expect(page.locator('#playing-view').locator('..').locator('.custom-select-dropdown'))
        .toHaveScreenshot(`${theme}-${size}-playing-guide-dropdown.png`);
    });
  }
}

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
        if (theme === 'spaceship-grey-light' && size === 'mobile' && name === 'recordings') {
          const cards = page.locator('.recording-item');
          await expect(cards).toHaveCount(3);
          for (const card of await cards.all()) {
            await expect(card.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();
            await expect(card.getByRole('link', { name: 'Archive', exact: true })).toBeVisible();
            const geometry = await card.evaluate(item => {
              const box = item.getBoundingClientRect();
              const [remove, archive] = Array.from(item.querySelectorAll('.recording-actions .btn'),
                action => action.getBoundingClientRect());
              return {
                topOffset: remove.top - archive.top,
                gap: archive.left - remove.right,
                leftInset: remove.left - box.left,
                rightInset: box.right - archive.right,
              };
            });
            expect(geometry.topOffset, 'Recording actions must share a top edge').toBeCloseTo(0, 1);
            expect(geometry.gap, 'Recording actions must not overlap').toBeGreaterThanOrEqual(0);
            expect(geometry.leftInset, 'Delete must remain inside its card').toBeGreaterThanOrEqual(0);
            expect(geometry.rightInset, 'Archive must remain inside its card').toBeGreaterThanOrEqual(0);
          }
        }
        if (theme === 'crt-starfield-1' && size === 'mobile' && name === 'configurations') {
          const geometry = await page.getByRole('link', { name: 'Manage ffmpeg profiles', exact: true }).evaluate(button => {
            const box = button.getBoundingClientRect();
            const actions = button.parentElement.getBoundingClientRect();
            const style = getComputedStyle(button);
            const expectedHeight = ['lineHeight', 'paddingTop', 'paddingBottom', 'borderTopWidth', 'borderBottomWidth']
              .reduce((height, property) => height + parseFloat(style[property]), 0);
            return {
              height: box.height,
              expectedHeight,
              topOffset: box.top - actions.top,
              leftOffset: box.left - actions.left,
              rightOverflow: box.right - actions.right,
            };
          });
          expect(geometry.height, 'Profile button must remain a single line at its CSS height').toBeCloseTo(geometry.expectedHeight, 1);
          expect(geometry.topOffset, 'Profile button must align with its action row').toBeCloseTo(0, 1);
          expect(geometry.leftOffset, 'Profile button must remain left-aligned').toBeCloseTo(0, 1);
          expect(geometry.rightOverflow, 'Profile button must fit its action row').toBeLessThanOrEqual(0);
        }
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
