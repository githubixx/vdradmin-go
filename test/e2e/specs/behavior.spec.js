import { test, expect, themes, selectTheme, openPage, screenshot, routeResources, assertWatchLayout, guideURL, prepareGuide, choosePlayingOption, assertGuideLayout } from './fixtures.js';

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

test('TV Guide switches layouts and preserves filters and channel through day navigation', async ({ page, request }) => {
  await prepareGuide(request);
  await openPage(page, guideURL.replace('&view=guide', ''));
  await expect(page.locator('#playing-view')).toHaveValue('list');
  await expect(page.locator('.playing-pager')).toBeHidden();
  await choosePlayingOption(page, 'playing-view', 'TV Guide');
  await expect(page.locator('.playing-guide')).toBeVisible();
  await expect(page.locator('#playing-start')).toHaveValue('18:00');
  await choosePlayingOption(page, 'channel-jump', 'Guide Channel 9');
  await expect(page.locator('#ch-9')).toBeFocused();
  await page.getByLabel('Start time (HH:MM)', { exact: true }).fill('19:00');
  await page.getByLabel('End time (HH:MM)', { exact: true }).focus();
  await expect(page).toHaveURL(/start=19%3A00/);
  await expect(page.locator('#channel-jump')).toHaveValue('ch-9');
  await page.getByRole('link', { name: 'Next day', exact: true }).click();
  const url = new URL(page.url());
  expect(url.searchParams.get('day')).toBe('2026-10-18');
  expect(url.searchParams.get('view')).toBe('guide');
  expect(url.searchParams.get('start')).toBe('19:00');
  expect(url.searchParams.get('end')).toBe('21:00');
  expect(url.searchParams.get('channel-jump')).toBe('ch-9');
  await expect(page.locator('#channel-jump')).toHaveValue('ch-9');
  await page.getByRole('link', { name: 'Previous day', exact: true }).click();
  await expect(page.locator('#ch-9')).toBeVisible();
  await choosePlayingOption(page, 'playing-view', 'List View');
  await expect(page.locator('.playing-pager')).toBeHidden();
  await expect(page.locator('.playing-group:visible')).toHaveCount(8);
});

test('TV Guide pages channels, restores history and adapts to viewport changes', async ({ page, request }) => {
  await prepareGuide(request);
  await openPage(page, guideURL);
  const previous = page.getByRole('button', { name: 'Previous channels', exact: true });
  const next = page.getByRole('button', { name: 'Next channels', exact: true });
  await expect(previous).toBeDisabled();
  const count = await page.locator('.playing-group:visible').count();
  const initialHeight = await page.locator('.playing-groups').evaluate(grid => grid.style.getPropertyValue('--guide-timeline-height'));
  expect(count).toBeGreaterThan(1);
  expect(count).toBeLessThan(9);
  await next.click();
  expect(await page.locator('.playing-groups').evaluate(grid => grid.style.getPropertyValue('--guide-timeline-height'))).toBe(initialHeight);
  await expect(page.locator(`#ch-${count + 1}`)).toBeFocused();
  const secondPage = page.url();
  await next.click();
  await page.goBack();
  await expect(page).toHaveURL(secondPage);
  await expect(page.locator(`#ch-${count + 1}`)).toBeVisible();
  await page.goForward();
  await choosePlayingOption(page, 'channel-jump', 'Guide Channel 9');
  await expect(next).toBeDisabled();
  await expect(page.locator('#ch-9')).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator('.playing-group:visible')).toHaveCount(1);
  await expect(page.locator('#ch-9')).toBeVisible();
  await assertGuideLayout(page);
  await page.reload();
  await expect(page.locator('#ch-9')).toBeVisible();
  await expect(page.locator('#channel-jump').locator('..').getByRole('combobox')).toHaveText('Guide Channel 9');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(page.locator('#ch-9')).toBeVisible();
  await expect(next).toBeDisabled();
  await previous.click();
  await expect(next).toBeEnabled();
});

test('TV Guide aligns two, three and one programmes within the same hour and fills missing data', async ({ page, request }) => {
  await prepareGuide(request);
  await openPage(page, guideURL.replace('end=21:00', 'end=19:00'));
  await assertGuideLayout(page);
  for (const [channel, count] of [[1, 2], [2, 3], [3, 1]]) {
    const group = page.locator(`.playing-group[aria-labelledby="ch-${channel}"]`);
    await expect(group.locator('.guide-slot')).toHaveCount(count);
    const durations = await group.locator('.guide-slot').evaluateAll(slots => slots.map(slot => Number(slot.dataset.guideDuration)));
    expect(durations).toEqual(Array(count).fill(3600 / count));
  }
  await expect(page.locator('.playing-group[aria-labelledby="ch-3"] time')).toHaveText('17:00-22:00');
  await expect(page.locator('.playing-group[aria-labelledby="ch-1"]')).not.toContainText('Late Night Culture');
  await choosePlayingOption(page, 'channel-jump', 'Guide Channel 5');
  const missing = page.locator('.playing-group[aria-labelledby="ch-5"]');
  await expect(missing.locator('.guide-gap')).toHaveCount(1);
  await expect(missing).toContainText('No programme information');
  await expect(missing.getByRole('button', { name: 'Record', exact: true })).toHaveCount(0);
  await expect(missing.locator('.guide-slot')).toHaveAttribute('data-guide-duration', '3600');
  await assertGuideLayout(page);
});

test('TV Guide keeps clipped short programmes readable on a shared scale', async ({ page, request }) => {
  await prepareGuide(request);
  await openPage(page, guideURL.replace('start=18:00', 'start=18:19').replace('end=21:00', 'end=18:21'));
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 844 });
    await choosePlayingOption(page, 'channel-jump', 'ZDF HD');
    const group = page.locator('.playing-group[aria-labelledby="ch-2"]');
    await expect(group.locator('.guide-slot')).toHaveCount(2);
    for (const slot of await group.locator('.guide-slot').all()) {
      await expect(slot).toHaveAttribute('data-guide-duration', '60');
    }
    await assertGuideLayout(page);
  }
});

test('TV Guide keeps the first row compact with dense off-page programmes and preserves global alignment', async ({ page, request }) => {
  for (const theme of ['metal-1', 'spaceship-cyan-dark']) {
    await selectTheme(request, theme);
    await prepareGuide(request, 'dense');
    await openPage(page, '/playing?day=2026-10-17&start=09:50&end=11:00&view=guide');
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      await choosePlayingOption(page, 'channel-jump', 'Das Erste HD');
      await assertGuideLayout(page);
      const first = page.locator('.playing-group[aria-labelledby="ch-1"] .guide-slot').first();
      const height = await first.evaluate(slot => slot.getBoundingClientRect().height);
      expect(height, 'Full subtitles must remain readable without restoring oversized first-row spacing').toBeLessThanOrEqual(400);
      await expect(first.locator('.channels-title')).toHaveText('Morning panorama with regional views');
      await expect(first.locator('.epg-subtitle')).toBeVisible();
      await expect(first.locator('.epg-subtitle')).toHaveText("Regional stories, landscapes and interviews with detailed background information about today's morning programme");
      const timelineHeight = await page.locator('.playing-groups').evaluate(grid =>
        parseFloat(grid.style.getPropertyValue('--guide-timeline-height')));
      await choosePlayingOption(page, 'channel-jump', 'Guide Channel 9');
      await assertGuideLayout(page);
      const boundary = await page.locator('.playing-group[aria-labelledby="ch-9"]').evaluate(group =>
        group.querySelectorAll('.guide-slot')[1].getBoundingClientRect().bottom -
        group.querySelector('.epg-list').getBoundingClientRect().top);
      expect(Math.abs(boundary - height), '10:00 must stay aligned across channel pages').toBeLessThanOrEqual(1);
      expect(await page.locator('.playing-groups').evaluate(grid =>
        parseFloat(grid.style.getPropertyValue('--guide-timeline-height')))).toBe(timelineHeight);
    }
  }
});

test('TV Guide keeps a full-day morning window compact despite an off-page one-minute programme', async ({ page, request }) => {
  for (const theme of ['metal-1', 'spaceship-cyan-dark']) {
    await selectTheme(request, theme);
    await prepareGuide(request, 'morning');
    await openPage(page, '/playing?day=2026-10-17&start=07:09&end=23:59&view=guide');
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      await choosePlayingOption(page, 'channel-jump', 'arte HD');
      await assertGuideLayout(page);
      const morning = page.locator('.playing-group[aria-labelledby="ch-3"] .guide-slot').first();
      await expect(morning).toContainText('ARD-Morgenmagazin');
      await expect(morning.locator('time')).toHaveText('05:30-09:00');
      const height = await morning.evaluate(slot => slot.getBoundingClientRect().height);
      expect(height, 'The 09:00 programme must be reachable without a giant morning card').toBeLessThanOrEqual(700);
      const timelineHeight = await page.locator('.playing-groups').evaluate(grid =>
        parseFloat(grid.style.getPropertyValue('--guide-timeline-height')));
      expect(timelineHeight, 'A clipped minute must not magnify the entire day').toBeLessThanOrEqual(2500);
      await choosePlayingOption(page, 'channel-jump', 'Guide Channel 9');
      await expect(page.locator('.playing-group[aria-labelledby="ch-9"] .guide-slot').first())
        .toHaveAttribute('data-guide-duration', '60');
      await assertGuideLayout(page);
      const boundary = await page.locator('.playing-group[aria-labelledby="ch-9"]').evaluate(group =>
        group.querySelectorAll('.guide-slot')[1].getBoundingClientRect().bottom -
        group.querySelector('.epg-list').getBoundingClientRect().top);
      expect(Math.abs(boundary - height), '09:00 must keep the same position on another channel page').toBeLessThanOrEqual(1);
      expect(await page.locator('.playing-groups').evaluate(grid =>
        parseFloat(grid.style.getPropertyValue('--guide-timeline-height')))).toBe(timelineHeight);
    }
  }
});

test('TV Guide preserves overlapping programme entries without covering their actions', async ({ page, request }) => {
  for (const theme of ['metal-1', 'spaceship-2']) {
    await selectTheme(request, theme);
    await prepareGuide(request, 'overlap');
    await openPage(page, guideURL.replace('end=21:00', 'end=19:00'));
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await assertGuideLayout(page);
      const group = page.locator('.playing-group[aria-labelledby="ch-1"]');
      await expect(group.locator('.guide-slot')).toHaveCount(3);
      const slots = await group.locator('.guide-slot').evaluateAll(slots => slots.map(slot => {
        const { left, right, top, bottom } = slot.getBoundingClientRect();
        return { left, right, top, bottom };
      }));
      for (let i = 0; i < slots.length; i++) {
        for (let j = i + 1; j < slots.length; j++) {
          const a = slots[i], b = slots[j];
          expect(a.right <= b.left + 1 || b.right <= a.left + 1 || a.bottom <= b.top + 1 || b.bottom <= a.top + 1,
            'Overlapping EPG entries must occupy separate lanes').toBeTruthy();
        }
      }
    }
  }
});

test('TV Guide supports keyboard selection, programme information and real recording actions', async ({ page, request }) => {
  await prepareGuide(request);
  await openPage(page, guideURL);
  const trigger = page.locator('#channel-jump').locator('..').getByRole('combobox');
  await trigger.focus();
  await page.keyboard.press('Enter');
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  await expect(page.locator('#ch-9')).toBeFocused();
  await page.keyboard.press('Shift+Tab');
  await choosePlayingOption(page, 'channel-jump', 'Das Erste HD');
  const first = page.locator('.playing-group:visible').first();
  await expect(first.getByRole('button', { name: 'Scheduled', exact: true })).toBeDisabled();
  const popupEvent = page.waitForEvent('popup');
  await first.getByRole('link', { name: 'Information', exact: true }).first().click();
  const popup = await popupEvent;
  await expect(popup.locator('.event-popup')).toBeVisible();
  await expect(popup.locator('.epg-subtitle')).toHaveText("A longer subtitle with details about this evening's programme");
  await popup.close();
  await first.getByRole('button', { name: 'Record', exact: true }).first().click();
  await expect(page).toHaveURL(/\/timers$/);
  await page.goBack();
  await page.reload();
  await expect(page.locator('.playing-group:visible').first().getByRole('button', { name: 'Scheduled', exact: true })).toHaveCount(2);
});

test('TV Guide handles empty schedules, invalid views and unrecognised channel anchors', async ({ page, request }) => {
  await prepareGuide(request, 'empty');
  await openPage(page, guideURL);
  await expect(page.getByText('No programs found', { exact: true })).toBeVisible();
  await expect(page.locator('.playing-pager')).toBeHidden();
  const invalid = await request.get('/playing?view=bogus');
  expect(invalid.status()).toBe(400);
  await selectTheme(request, 'metal-1');
  await prepareGuide(request);
  await openPage(page, `${guideURL}&channel-jump=missing#%ZZ`);
  await expect(page.locator('#ch-1')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Previous channels', exact: true })).toBeDisabled();
});

test('TV Guide remains readable without JavaScript and hides recording actions from viewers', async ({ browser, request }) => {
  await prepareGuide(request);
  const response = await request.get(guideURL, {
    headers: { Authorization: `Basic ${Buffer.from('viewer:test-password').toString('base64')}` },
  });
  expect(response.ok()).toBeTruthy();
  const html = await response.text();
  expect(html).toContain('Information');
  expect(html).not.toContain('action="/timers/create"');
  const context = await browser.newContext({
    javaScriptEnabled: false,
    httpCredentials: { username: 'admin', password: 'test-password' },
    baseURL: 'http://127.0.0.1:18763',
  });
  try {
    const page = await context.newPage();
    await routeResources(page, []);
    await page.goto(guideURL);
    await expect(page.locator('.playing-group:visible')).toHaveCount(9);
    await page.locator('#playing-view').selectOption('list');
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.locator('.playing-guide')).toHaveCount(0);
  } finally {
    await context.close();
  }
});

for (const colorScheme of ['light', 'dark']) {
  test(`system ${colorScheme} TV Guide follows browser preference`, async ({ page, request }) => {
    await selectTheme(request, 'system');
    await prepareGuide(request);
    await page.emulateMedia({ colorScheme });
    await openPage(page, guideURL);
    await expect(page.locator('html')).not.toHaveAttribute('data-theme');
    await assertGuideLayout(page);
    const before = await page.locator('body').evaluate(body => getComputedStyle(body).backgroundColor);
    await page.emulateMedia({ colorScheme: colorScheme === 'light' ? 'dark' : 'light' });
    await expect.poll(() => page.locator('body').evaluate(body => getComputedStyle(body).backgroundColor)).not.toBe(before);
    await page.setViewportSize({ width: 390, height: 844 });
    await assertGuideLayout(page);
  });
}

for (const theme of themes) {
  test(`${theme} TV Guide fits desktop, intermediate and mobile widths`, async ({ page, request }) => {
    await selectTheme(request, theme);
    await prepareGuide(request);
    await openPage(page, guideURL);
    for (const width of [1440, 769, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      await assertGuideLayout(page);
      await choosePlayingOption(page, 'channel-jump', 'Guide Channel 9');
      await expect(page.locator('#ch-9')).toBeFocused();
      await assertGuideLayout(page);
      await choosePlayingOption(page, 'channel-jump', 'Das Erste HD');
    }
  });
}

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
