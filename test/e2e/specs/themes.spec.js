import { test, expect, themes, matrixThemes, representatives, selectTheme, openPage, screenshot, assertWatchLayout, guideURL, prepareGuide, assertGuideLayout } from './fixtures.js';

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
      if ((theme === 'light' || theme === 'dark') && size === 'mobile') {
        const record = page.locator('.playing-group:visible').getByRole('button', { name: 'Record', exact: true }).first();
        await expect(record).toBeVisible();
        await expect(record).toBeEnabled();
        await expect(record).toHaveCSS('background-color',
          theme === 'light' ? 'rgb(124, 58, 237)' : 'rgb(167, 139, 250)');
        await expect(record).toHaveCSS('color', 'rgb(255, 255, 255)');
        await expect(record).toHaveCSS('border-radius', '999px');
      }
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

for (const theme of matrixThemes) {
  for (const [size, viewport] of Object.entries(sizes)) {
    test(`${theme} ${size} reference pages`, async ({ page, request }) => {
      await selectTheme(request, theme);
      await page.setViewportSize(viewport);
      await openPage(page, '/channels?day=2026-10-17');
      await expect(page.locator('.epg-item').first()).toBeVisible();
      if (theme === 'retro-arcade' && size === 'mobile') {
        const channel = page.locator('#channel').locator('..').locator('.custom-select-trigger');
        const day = page.locator('#day').locator('..').locator('.custom-select-trigger');
        await expect(channel).toBeVisible();
        await expect(channel).toContainText('Das Erste HD');
        await expect(day).toBeVisible();
        await expect(day).toContainText(/\b\d{4}-\d{2}-\d{2}\b/);
      }
      if (theme === 'spaceship-blue-light' && size === 'mobile') {
        const cards = page.locator('.epg-item');
        await expect(cards).toHaveCount(3);
        for (const card of await cards.all()) {
          const information = card.getByRole('link', { name: 'Information', exact: true });
          await expect(information).toBeVisible();
          await expect(information).toHaveAttribute('href', /^\/event\?channel=.+&id=\d+$/);
          await expect(information).toHaveCSS('color', 'rgb(239, 246, 255)');
          await expect(information).toHaveCSS('border-color', 'rgba(96, 165, 250, 0.8)');
          await expect(information).toHaveCSS('border-radius', '999px');
          await expect(information).toHaveCSS('box-shadow', /rgba\(96, 165, 250, 0\.6\)/);
          const geometry = await card.evaluate(item => {
            const box = item.getBoundingClientRect();
            const information = item.querySelector('.epg-actions > a').getBoundingClientRect();
            const record = item.querySelector('.epg-actions button').getBoundingClientRect();
            return {
              leftInset: information.left - box.left,
              rightInset: box.right - record.right,
              bottomInset: box.bottom - information.bottom,
              gap: record.left - information.right,
              topDifference: information.top - record.top,
              heightDifference: information.height - record.height,
            };
          });
          expect(geometry.leftInset).toBeGreaterThanOrEqual(0);
          expect(geometry.rightInset).toBeGreaterThanOrEqual(0);
          expect(geometry.bottomInset).toBeGreaterThanOrEqual(0);
          expect(geometry.gap).toBeGreaterThanOrEqual(0);
          expect(geometry.topDifference).toBeCloseTo(0, 1);
          expect(geometry.heightDifference).toBeCloseTo(0, 1);
        }
        await expect(page.getByLabel('Menu', { exact: true })).toBeVisible();
        const icon = await page.locator('.nav-burger-button .burger-icon').evaluate(element => {
          const style = getComputedStyle(element);
          return {
            width: style.width, height: style.height,
            middle: style.backgroundImage, middleSize: style.backgroundSize,
            edges: ['::before', '::after'].map(pseudo => {
              const edge = getComputedStyle(element, pseudo);
              return { height: edge.height, color: edge.backgroundColor };
            }),
          };
        });
        expect(icon.width).toBe('18px');
        expect(icon.height).toBe('12px');
        expect(icon.middle).toContain('rgb(239, 246, 255)');
        expect(icon.middleSize).toBe('100% 2px');
        expect(icon.edges).toEqual(Array(2).fill({ height: '2px', color: 'rgb(239, 246, 255)' }));
      }
      await screenshot(page, `${theme}-${size}-channels`);
      await openPage(page, '/timers?day=2026-10-17');
      await expect(page.locator('.timer-item').first()).toBeVisible();
      if (theme === 'leather-style-red-1' && size === 'desktop') {
        for (const [state, color] of [
          ['recording', 'rgb(74, 222, 128)'],
          ['collision', 'rgb(251, 191, 36)'],
          ['conflict', 'rgb(255, 82, 100)'],
        ]) {
          await expect(page.locator(`.timer-item.${state}`).first()).toHaveCSS('border-left-color', color);
        }
      }
      if (theme === 'mantle-1' && size === 'mobile') {
        const day = page.locator('#day').locator('..').locator('.custom-select-trigger');
        await expect(day).toBeVisible();
        await expect(day).toContainText(/\b\d{4}-\d{2}-\d{2}\b/);
        await expect(day).toHaveCSS('border-radius', '999px');
        await expect(day).toHaveCSS('box-shadow', /inset/);
        await expect(page.getByLabel('Menu', { exact: true })).toBeVisible();
        const icon = page.locator('.nav-burger-button .burger-icon');
        await expect(icon).toHaveCSS('width', '18px');
        await expect(icon).toHaveCSS('height', '12px');
        await expect(icon).toHaveCSS('background-image', /linear-gradient/);
        await expect(page.locator('.timer-item.recording').first()).toHaveCSS('border-left-color', 'rgb(88, 182, 148)');
        await expect(page.locator('.timer-item.collision').first()).toHaveCSS('border-left-color', 'rgb(204, 155, 99)');
        await expect(page.locator('.timer-item.conflict').first()).toHaveCSS('border-left-color', 'rgb(182, 98, 123)');
        await expect(page.locator('.timeline-block.critical').first()).toBeVisible();
      }
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
        if (name === 'watch') {
          await assertWatchLayout(page);
          if (theme === 'spaceship-blue-light' && size === 'desktop') {
            const sizeControl = page.locator('#watchtv-size').locator('..').locator('.custom-select-trigger');
            await expect(sizeControl).toBeVisible();
            await expect(sizeControl).toContainText('1:2');
            await expect(sizeControl).toHaveCSS('border-radius', '10.4px');
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

test('spaceship-grey-light mobile recording actions remain aligned', async ({ page, request }) => {
  await selectTheme(request, 'spaceship-grey-light');
  await page.setViewportSize(sizes.mobile);
  await openPage(page, '/recordings');
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
  await screenshot(page, 'spaceship-grey-light-mobile-recordings');
});

for (const theme of ['spaceship-grey-dark', 'spaceship-grey-light']) {
  test(`${theme} desktop Watch layout`, async ({ page, request }) => {
    await selectTheme(request, theme);
    await page.setViewportSize(sizes.desktop);
    await openPage(page, '/watch?interval=0');
    await assertWatchLayout(page);
    await screenshot(page, `${theme}-desktop-watch`);
  });
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
