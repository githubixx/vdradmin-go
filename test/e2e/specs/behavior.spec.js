import { test, expect, themes, selectTheme, openPage, screenshot, routeResources, assertWatchLayout } from './fixtures.js';

test.beforeEach(async ({ request }) => {
  await selectTheme(request, 'metal-1');
});

test('menu preserves navigation, active link, Escape focus and outside click', async ({ page }) => {
  await openPage(page, '/channels?day=2026-10-17');
  const menu = page.getByLabel('Menu', { exact: true });
  await menu.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('link', { name: 'Channels', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('link', { name: 'Jobs', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Recordings', exact: true })).toHaveAttribute('href', '/recordings');
  if (test.info().project.name === 'chromium') await screenshot(page, 'metal-menu-open');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('link', { name: 'Jobs', exact: true })).not.toBeVisible();
  await expect(menu).toBeFocused();
  await menu.click();
  await page.locator('main').click({ position: { x: 5, y: 5 } });
  await expect(page.getByRole('link', { name: 'Jobs', exact: true })).not.toBeVisible();
});

test('Aurora Glass aligns shared navigation and archive-job actions', async ({ page, request }) => {
  await selectTheme(request, 'aurora-glass-1');
  await openPage(page, '/recordings/archive/jobs');
  const layout = await page.evaluate(() => {
    const badge = document.querySelector('.nav-user .badge').getBoundingClientRect();
    const menu = document.querySelector('.nav-burger-button').getBoundingClientRect();
    const grid = document.createElement('div');
    grid.className = 'config-grid';
    grid.innerHTML = '<label>Actions</label><div class="archive-job-actions"><a class="btn btn-sm btn-secondary">Jobs</a><button class="btn btn-sm btn-danger">Cancel</button></div>';
    document.querySelector('main').append(grid);
    const [jobs, cancel] = grid.querySelectorAll('.archive-job-actions > *');
    return {
      navigationCenterDifference: Math.abs((badge.top + badge.bottom) - (menu.top + menu.bottom)),
      actionTopDifference: Math.abs(jobs.getBoundingClientRect().top - cancel.getBoundingClientRect().top),
    };
  });
  expect(layout.navigationCenterDifference, 'Admin badge and burger menu should be vertically centered').toBeLessThanOrEqual(1);
  expect(layout.actionTopDifference, 'Jobs and Cancel should share a top edge').toBeLessThanOrEqual(1);
});

test('explicit theme wins over browser preference and legacy cookie', async ({ page, context }) => {
  await context.addCookies([{ name: 'theme', value: 'light', url: 'http://127.0.0.1:18763' }]);
  await page.emulateMedia({ colorScheme: 'dark' });
  await openPage(page, '/channels?day=2026-10-17');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'metal-1');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'metal-1');
});

test('theme options and enhanced selects are available', async ({ page }) => {
  await openPage(page, '/configurations');
  const themeSelect = page.locator('select[name="ui_theme"]');
  await expect(themeSelect.locator('option')).toHaveCount(38);
  await expect(themeSelect).toHaveValue('metal-1');
  const trigger = themeSelect.locator('..').locator('.custom-select-trigger');
  await trigger.click();
  await expect(themeSelect.locator('..').locator('.custom-select-dropdown').getByText('Metal 1', { exact: true })).toBeVisible();
  if (test.info().project.name === 'chromium') await screenshot(page, 'metal-theme-dropdown-open');
  await page.getByRole('heading', { name: 'General Settings', exact: true }).click();
  await expect(themeSelect.locator('..').locator('.custom-select-dropdown')).not.toHaveClass(/open/);
});

test('applying and saving a theme uses the real configuration handlers', async ({ page }) => {
  await openPage(page, '/configurations');
  const themeSelect = page.locator('select[name="ui_theme"]');
  const selectContainer = themeSelect.locator('..');
  await selectContainer.locator('.custom-select-trigger').click();
  await selectContainer.locator('.custom-select-dropdown').getByText('Light', { exact: true }).click();
  await page.getByRole('button', { name: 'Apply', exact: true }).first().click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(page.locator('select[name="ui_theme"]')).toHaveValue('light');
});

test('recording filters make real htmx requests and preserve results', async ({ page }) => {
  await openPage(page, '/recordings');
  await expect(page.locator('.recording-item').first()).toBeVisible();
  const response = page.waitForResponse(response =>
    response.url().includes('/recordings') && response.request().headers()['hx-request'] === 'true');
  await page.getByRole('button', { name: 'Refresh recordings', exact: true }).click();
  expect((await response).ok()).toBeTruthy();
  await expect(page.locator('.recording-item').first()).toBeVisible();
  await expect(page.locator('.custom-select-container .custom-select-container')).toHaveCount(0);
});

test('htmx fragment selects initialize exactly once', async ({ page }) => {
  await openPage(page, '/recordings');
  await page.evaluate(() => {
    const fragment = document.createElement('div');
    fragment.id = 'fragment-probe';
    const select = document.createElement('select');
    select.setAttribute('aria-label', 'Fragment choice');
    select.add(new Option('First choice', 'first'));
    select.add(new Option('Second choice', 'second'));
    fragment.append(select);
    document.querySelector('main').append(fragment);
    window.htmx.trigger(fragment, 'htmx:load', { elt: fragment });
    window.htmx.trigger(fragment, 'htmx:load', { elt: fragment });
  });
  const fragment = page.locator('#fragment-probe');
  await expect(fragment.locator('.custom-select-trigger')).toHaveCount(1);
  await fragment.locator('.custom-select-trigger').click();
  await fragment.locator('.custom-select-dropdown').getByText('Second choice', { exact: true }).click();
  await expect(fragment.getByRole('combobox', { name: 'Fragment choice', exact: true })).toHaveCount(1);
  await expect(fragment.locator('select')).toHaveValue('second');
});

test('mobile menu remains usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openPage(page, '/channels?day=2026-10-17');
  await page.getByLabel('Menu', { exact: true }).click();
  await page.getByRole('link', { name: 'Recordings', exact: true }).click();
  await expect(page).toHaveURL(/\/recordings$/);
  await expect(page.locator('.recording-item').first()).toBeVisible();
});

test('timer fixtures expose recording, inactive and conflict states', async ({ page }) => {
  await openPage(page, '/timers?day=2026-10-17');
  await expect(page.getByRole('button', { name: 'Recording', exact: true }).first()).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Enable', exact: true }).first()).toBeVisible();
  await expect(page.locator('.timer-item.collision').first()).toBeVisible();
  await expect(page.locator('.timer-item.conflict').first()).toBeVisible();
});

for (const theme of themes) {
  test(`${theme} programme columns align across time ranges`, async ({ page, request }, testInfo) => {
    await selectTheme(request, theme);
    const response = await request.get('/__test/ready');
    expect(response.ok()).toBeTruthy();
    const { urls } = await response.json();
    for (const width of [1440, 769, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      for (const route of ['channels', 'playing']) {
        await openPage(page, urls[route]);
        const rows = await page.locator('.epg-item').evaluateAll((items, viewportWidth) => {
          const ranges = ['11:11-11:11', '20:48-23:59', '17:11-18:13', '19:13-20:14', '21:14-22:56'];
          items.forEach((item, index) => {
            item.querySelector('.epg-time time').textContent = ranges[index % ranges.length];
          });
          if (viewportWidth === 769) {
            items[0].querySelector('.epg-content h3').textContent =
              'A long programme title with several words that must wrap naturally without overlapping the time column or the action buttons';
          }
          return items.map(item => {
            const time = item.querySelector('.epg-time time');
            const content = item.querySelector('.epg-content');
            const title = content.querySelector('h3, h4');
            const timeBox = time.getBoundingClientRect();
            const contentBox = content.getBoundingClientRect();
            const titleBox = title.getBoundingClientRect();
            const range = document.createRange();
            range.selectNodeContents(time);
            const titleRange = document.createRange();
            titleRange.selectNodeContents(title);
            const titleLines = [...titleRange.getClientRects()];
            return {
              titleLeft: titleBox.left, contentLeft: contentBox.left,
              titleRight: titleBox.right, contentRight: contentBox.right,
              timeRight: timeBox.right, textRight: range.getBoundingClientRect().right,
              timeBottom: timeBox.bottom, contentTop: contentBox.top,
              titleTextRight: Math.max(...titleLines.map(line => line.right)),
              titleLines: titleLines.length,
            };
          });
        }, width);
        expect(rows.length, `${route} needs multiple programmes`).toBeGreaterThan(1);
        if (width > 768) {
          const lefts = rows.map(row => row.titleLeft);
          expect(Math.max(...lefts) - Math.min(...lefts), `${theme} ${route} title column`).toBeLessThanOrEqual(0.5);
          for (const row of rows) {
            expect(row.textRight, 'Time text must fit its reserved column').toBeLessThanOrEqual(row.timeRight + 0.5);
            expect(row.textRight, 'Time must not overlap the title').toBeLessThan(row.contentLeft);
            expect(row.titleRight, 'Title must fit the content column').toBeLessThanOrEqual(row.contentRight + 0.5);
            expect(row.titleTextRight, 'Title text must not overflow its column').toBeLessThanOrEqual(row.contentRight + 0.5);
          }
          if (width === 769) expect(rows[0].titleLines, 'Long titles must wrap at the desktop breakpoint').toBeGreaterThan(1);
        } else if (theme !== 'spaceship-2') {
          for (const row of rows) {
            expect(row.contentTop, 'Mobile content must remain below its time').toBeGreaterThanOrEqual(row.timeBottom);
          }
        }
        if (theme === 'spaceship-purple-dark') {
          await page.screenshot({ path: testInfo.outputPath(`${route}-${width}-alignment.png`), fullPage: true });
        }
      }
    }
  });
}

for (const width of [1440, 390]) {
  test(`Metal timer conflict indicators use semantic colors at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 });
    await openPage(page, '/timers?day=2026-10-17');
    for (const [state, color] of [
      ['collision:not(.conflict)', 'rgb(245, 158, 11)'],
      ['conflict', 'rgb(239, 68, 68)'],
    ]) {
      const items = page.locator(`.timer-item.${state}`);
      expect(await items.count()).toBeGreaterThan(0);
      for (const item of await items.all()) {
        for (const side of ['top', 'right', 'bottom', 'left']) {
          await expect(item).toHaveCSS(`border-${side}-color`, color);
        }
      }
    }
    for (const [state, border, background] of [
      ['collision', 'rgb(245, 158, 11)', 'rgba(245, 158, 11, 0.24)'],
      ['critical', 'rgb(239, 68, 68)', 'rgba(239, 68, 68, 0.24)'],
    ]) {
      const blocks = page.locator(`.timeline-block.${state}`);
      expect(await blocks.count()).toBeGreaterThan(0);
      for (const block of await blocks.all()) {
        await expect(block).toHaveCSS('border-left-color', border);
        await expect(block).toHaveCSS('background-color', background);
      }
    }
    await expect(page.locator('.timer-item.recording').first()).toHaveCSS('border-left-color', 'rgb(229, 231, 235)');
    await expect(page.locator('.timer-item.conflict .btn-danger').first()).toHaveCSS('background-color', 'rgb(229, 231, 235)');
    await page.screenshot({ path: testInfo.outputPath('metal-conflict-indicators.png'), fullPage: true });
  });
}

test('Watch TV panels remain separate at every snapshot size', async ({ page, request }) => {
  for (const theme of ['metal-1', 'spaceship-2']) {
    await selectTheme(request, theme);
    for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
      await page.setViewportSize(viewport);
      for (const size of ['full', 'half', 'quarter']) {
        await openPage(page, `/watch?interval=0&size=${size}`);
        await assertWatchLayout(page);
      }
    }
  }
});

test('viewer navigation omits administrator actions', async ({ browser, baseURL }) => {
  const context = await browser.newContext({
    baseURL,
    httpCredentials: { username: 'viewer', password: 'test-password' },
    extraHTTPHeaders: {
      Authorization: `Basic ${Buffer.from('viewer:test-password').toString('base64')}`,
    },
  });
  try {
    const failures = [];
    await routeResources(context, failures);
    const page = await context.newPage();
    await page.goto('/channels?day=2026-10-17');
    await page.getByLabel('Menu', { exact: true }).click();
    await expect(page.getByRole('link', { name: 'Jobs', exact: true })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Recordings', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Record', exact: true })).toHaveCount(0);
    expect(failures).toEqual([]);
  } finally {
    await context.close();
  }
});

test('native controls and menu work without JavaScript', async ({ browser, baseURL }) => {
  const context = await browser.newContext({
    baseURL,
    javaScriptEnabled: false,
    httpCredentials: { username: 'admin', password: 'test-password' },
  });
  try {
    const failures = [];
    await routeResources(context, failures);
    const page = await context.newPage();
    await page.goto('/channels?day=2026-10-17');
    await expect(page.locator('select[name="channel"]')).toBeVisible();
    await page.getByLabel('Menu', { exact: true }).click();
    await expect(page.getByRole('link', { name: 'Channels', exact: true })).toBeVisible();
    expect(failures).toEqual([]);
  } finally {
    await context.close();
  }
});
