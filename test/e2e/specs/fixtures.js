import { test as base, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

const resourceRoot = path.resolve(import.meta.dirname, '../../../.cache/e2e-resources');
const resources = JSON.parse(await readFile(new URL('../resources.lock.json', import.meta.url), 'utf8'));

export const screenshotComparisonOptions = Object.freeze({
  'aurora-glass-1-mobile-timers': { maxDiffPixels: 100 },
  'luxury-1-mobile-timers': { maxDiffPixels: 100 },
  'spaceship-grey-dark-desktop-watch': { maxDiffPixels: 12 },
  'spaceship-grey-light-desktop-watch': { maxDiffPixels: 12 },
  'crt-starfield-1-mobile-configurations': { maxDiffPixels: 3600 },
  'spaceship-grey-light-mobile-recordings': { threshold: 0.02, maxDiffPixels: 450 },
  'light-mobile-playing-guide': { maxDiffPixels: 40 },
  'spaceship-blue-light-mobile-channels': { threshold: 0.02, maxDiffPixels: 1050 },
});

export const test = base.extend({
  page: async ({ page }, use) => {
    const failures = [];
    page.on('pageerror', error => failures.push(error.message));
    page.on('response', response => {
      if (response.status() >= 400) failures.push(`${response.status()} ${response.url()}`);
    });
    page.on('requestfailed', request => {
      if (request.failure()?.errorText !== 'net::ERR_ABORTED') {
        failures.push(`${request.failure()?.errorText} ${request.url()}`);
      }
    });
    await routeResources(page, failures);
    await page.clock.install({ time: new Date('2026-10-17T12:00:00Z') });
    await use(page);
    expect(failures, 'Browser errors and unavailable resources').toEqual([]);
  },
});

export async function routeResources(target, failures) {
    await target.route('https://**/*', async route => {
      const url = new URL(route.request().url()).href;
      const resource = resources[url];
      if (!resource) {
        failures.push(`Unregistered external resource: ${url}`);
        await route.abort('blockedbyclient');
        return;
      }
      await route.fulfill({
        contentType: resource.contentType,
        body: await readFile(path.join(resourceRoot, resource.sha256)),
        headers: { 'access-control-allow-origin': '*' },
      });
    });
}

export { expect };

export const themes = [
  'aurora-glass-1', 'cartoon-1', 'crt-starfield-1', 'dark', 'fitness',
  'glas-1', 'gold-1', 'golden-moon', 'light', 'lighting-1', 'luxury-1',
  'mantle-1', 'metal-1', 'retro-arcade', 'solar-system-1', 'space-night-1',
  'spaceship-2',
  ...['blue', 'cyan', 'green', 'grey', 'magenta', 'mono', 'orange', 'purple', 'red', 'yellow']
    .flatMap(color => [`spaceship-${color}-dark`, `spaceship-${color}-light`]),
];

export const representatives = [
  'light', 'dark', 'cartoon-1', 'metal-1', 'spaceship-grey-dark',
  'spaceship-grey-light', 'spaceship-2', 'crt-starfield-1',
];

export async function selectTheme(request, theme) {
  const reset = await request.post('/__test/reset');
  expect(reset.ok(), `Reset fixtures: ${await reset.text()}`).toBeTruthy();
  const response = await request.post('/__test/theme', { data: { theme } });
  expect(response.ok(), `Select theme ${theme}: ${await response.text()}`).toBeTruthy();
}

export async function openPage(page, url) {
  if (url.startsWith('/timers?day=')) {
    const readiness = await page.request.get('/__test/ready');
    expect(readiness.ok()).toBeTruthy();
    url = (await readiness.json()).urls.timers;
  }
  const watchRequests = url.startsWith('/watch?')
    ? Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === '/watch/snapshot'),
      page.waitForResponse(response => new URL(response.url()).pathname === '/watch/now'),
    ])
    : null;
  const response = await page.goto(url);
  expect(response?.status()).toBe(200);
  if (watchRequests) {
    for (const result of await watchRequests) expect(result.ok()).toBeTruthy();
  }
  await expect(page.locator('main')).toBeVisible();
  await waitForPageResources(page);
}

export async function waitForPageResources(page) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.all(Array.from(document.images, async image => {
      if (!image.complete) {
        await new Promise((resolve, reject) => {
          image.addEventListener('load', resolve, { once: true });
          image.addEventListener('error', reject, { once: true });
        });
      }
      if (image.naturalWidth === 0) throw new Error(`Image failed to load: ${image.src}`);
    }));
  });
}

export async function assertWatchLayout(page) {
  const layout = await page.evaluate(() => {
    const box = selector => {
      const element = document.querySelector(selector);
      if (!element) throw new Error(`Missing Watch TV layout element: ${selector}`);
      const { left, top, right, bottom, width, height } = element.getBoundingClientRect();
      return { left, top, right, bottom, width, height };
    };
    return {
      wide: matchMedia('(min-width: 1101px)').matches,
      viewportWidth: innerWidth,
      gridContentWidth: document.querySelector('.watchtv-grid').scrollWidth,
      grid: box('.watchtv-grid'),
      screenColumn: box('.watchtv-screen'),
      screen: box('#watchtv-screen'),
      remote: box('.remote-panel'),
      channels: box('.watchtv-channels-panel'),
      programme: box('#watchtv-now'),
    };
  });
  expect(layout.screen.right, 'Screen must fit its own grid column').toBeLessThanOrEqual(layout.screenColumn.right + 1);
  expect(layout.grid.right, 'Watch TV grid must fit the viewport').toBeLessThanOrEqual(layout.viewportWidth + 1);
  expect(layout.gridContentWidth, 'Watch TV content must not overflow its grid').toBeLessThanOrEqual(layout.grid.width + 1);
  for (const panel of [layout.screen, layout.remote, layout.channels, layout.programme]) {
    expect(panel.width, 'Watch TV panels must have a visible width').toBeGreaterThan(0);
    expect(panel.left, 'Watch TV panels must stay inside the grid').toBeGreaterThanOrEqual(layout.grid.left - 1);
    expect(panel.right, 'Watch TV panels must stay inside the grid').toBeLessThanOrEqual(layout.grid.right + 1);
  }

  if (layout.wide) {
    expect(layout.screen.right, 'Screen must not cover the remote').toBeLessThanOrEqual(layout.remote.left);
    expect(layout.remote.right, 'Remote must not cover the channel list').toBeLessThanOrEqual(layout.channels.left);
    expect(Math.abs(layout.screen.top - layout.remote.top)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.screen.top - layout.channels.top)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.screen.height - layout.remote.height)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.screen.height - layout.channels.height)).toBeLessThanOrEqual(1);
  } else {
    expect(layout.remote.top, 'Mobile remote must follow the screen').toBeGreaterThanOrEqual(layout.screen.bottom);
    expect(layout.channels.top, 'Mobile channel list must follow the remote').toBeGreaterThanOrEqual(layout.remote.bottom);
  }
  expect(layout.programme.top, 'Programme information must be below every panel')
    .toBeGreaterThanOrEqual(Math.max(layout.screen.bottom, layout.remote.bottom, layout.channels.bottom));
  expect(Math.abs(layout.programme.width - layout.grid.width)).toBeLessThanOrEqual(1);
}

export const guideURL = '/playing?day=2026-10-17&start=18:00&end=21:00&view=guide';

export async function prepareGuide(request, scenario = 'many') {
  const response = await request.post('/__test/guide', { data: { scenario } });
  expect(response.ok(), await response.text()).toBeTruthy();
}

export async function choosePlayingOption(page, id, label) {
  const wrapper = page.locator(`#${id}`).locator('..');
  await wrapper.locator('.custom-select-trigger').click();
  await wrapper.getByRole('option', { name: label, exact: true }).click();
}

export async function assertGuideLayout(page) {
  await expect(page.locator('.playing-pager')).toBeVisible();
  await expect(page.locator('.playing-pager')).toHaveClass(/\bui-pager\b/);
  await expect(page.locator('.playing-pager')).toHaveCSS('display', 'flex');
  await expect(page.locator('.playing-pager')).toHaveCSS('justify-content', 'center');
  await expect(page.locator('.playing-guide')).toHaveClass(/guide-timeline-ready/);
  await expect.poll(() => page.locator('.playing-group:visible').evaluateAll(groups =>
    new Set(groups.map(group => Math.round(group.getBoundingClientRect().top))).size),
  { message: 'Responsive pagination must settle to one row of channel columns' }).toBe(1);
  const layout = await page.evaluate(() => {
    const grid = document.querySelector('.playing-groups');
    const visible = [...grid.querySelectorAll('.playing-group:not([hidden])')];
    const rect = element => {
      const { left, right, top, bottom, width } = element.getBoundingClientRect();
      return { left, right, top, bottom, width };
    };
    return {
      viewport: innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      grid: rect(grid),
      columns: visible.map(rect),
      timelines: visible.map(group => {
        const list = group.querySelector('.epg-list');
        const box = rect(list);
        return {
          ...box,
          slots: [...list.querySelectorAll('.guide-slot')].map(slot => ({
            ...rect(slot),
            offset: Number(slot.dataset.guideOffset),
            duration: Number(slot.dataset.guideDuration),
          })),
        };
      }),
      duration: Number(grid.dataset.guideDuration),
      cards: visible.flatMap(group => [...group.querySelectorAll('.epg-item')].map(card => ({
        box: rect(card), scrollWidth: card.scrollWidth, clientWidth: card.clientWidth,
        scrollHeight: card.scrollHeight, clientHeight: card.clientHeight,
        time: rect(card.querySelector('.epg-time')),
        content: rect(card.querySelector('.epg-content')),
        actions: [...card.querySelectorAll('.epg-actions .btn')].map(rect),
      }))),
      toolbar: [...document.querySelectorAll('.playing-toolbar .custom-select-trigger, .playing-toolbar .btn, .playing-toolbar input[type="time"]')].map(rect),
    };
  });
  expect(layout.documentWidth, 'Guide must not overflow horizontally').toBeLessThanOrEqual(layout.viewport);
  expect(layout.columns.length).toBeGreaterThan(0);
  for (let i = 0; i < layout.columns.length; i++) {
    const column = layout.columns[i];
    expect(column.left).toBeGreaterThanOrEqual(layout.grid.left - 1);
    expect(column.right).toBeLessThanOrEqual(layout.grid.right + 1);
    if (i) expect(column.left).toBeGreaterThanOrEqual(layout.columns[i - 1].right);
    expect(Math.abs(column.top - layout.columns[0].top)).toBeLessThanOrEqual(1);
  }
  const boundaries = [];
  for (const timeline of layout.timelines) {
    expect(Math.abs(timeline.top - layout.timelines[0].top), 'All channels must start at the same time').toBeLessThanOrEqual(1);
    expect(Math.abs(timeline.bottom - layout.timelines[0].bottom), 'All channels must end at the same time').toBeLessThanOrEqual(1);
    boundaries.push({ time: 0, position: 0 }, { time: layout.duration, position: timeline.bottom - timeline.top });
    for (const slot of timeline.slots) {
      boundaries.push(
        { time: slot.offset, position: slot.top - timeline.top },
        { time: slot.offset + slot.duration, position: slot.bottom - timeline.top },
      );
      expect(slot.top).toBeGreaterThanOrEqual(timeline.top - 1);
      expect(slot.bottom).toBeLessThanOrEqual(timeline.bottom + 1);
      for (const later of timeline.slots.filter(later => later.offset >= slot.offset + slot.duration)) {
        expect(slot.bottom, 'Chronological slots must not overlap').toBeLessThanOrEqual(later.top + 1);
      }
    }
    boundaries.sort((a, b) => a.time - b.time);
    for (let i = 1; i < boundaries.length; i++) {
      const previous = boundaries[i - 1], current = boundaries[i];
      if (current.time === previous.time) {
        expect(Math.abs(current.position - previous.position), 'Matching times must align across channels').toBeLessThanOrEqual(1);
      } else {
        expect(current.position, 'The shared time axis must remain chronological').toBeGreaterThanOrEqual(previous.position - 1);
      }
    }
    expect(Math.abs(Math.max(...timeline.slots.map(slot => slot.bottom)) - timeline.bottom), 'Every channel must fill the selected window').toBeLessThanOrEqual(1);
  }
  for (const card of layout.cards) {
    expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth + 1);
    expect(card.scrollHeight, 'Short programme cards must remain readable without internal scrolling').toBeLessThanOrEqual(card.clientHeight + 1);
    expect(card.content.top, 'Content must follow the time').toBeGreaterThanOrEqual(card.time.bottom - 1);
    for (const action of card.actions) {
      expect(action.left).toBeGreaterThanOrEqual(card.box.left - 1);
      expect(action.right).toBeLessThanOrEqual(card.box.right + 1);
      expect(action.top, 'Actions must follow programme content').toBeGreaterThanOrEqual(card.content.bottom - 1);
    }
  }
  for (const control of layout.toolbar) {
    expect(control.left).toBeGreaterThanOrEqual(0);
    expect(control.right).toBeLessThanOrEqual(layout.viewport + 1);
  }
}
export async function screenshot(page, name) {
  expect(process.env.E2E_SCREENSHOT_RUNTIME, 'Generate and compare screenshots through scripts/test-e2e.sh')
    .toBe('playwright-1.64.1-noble');
  const readiness = await page.request.get('/__test/ready');
  expect(readiness.ok()).toBeTruthy();
  const metadata = await readiness.json();
  await page.evaluate(({ storagePathNormalization }) => {
    const normalize = text => text
      .replace(/\b(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)(?= \d{4}-\d{2}-\d{2})/g, 'Sun')
      .replace(/\b\d{4}-\d{2}-\d{2}\b/g, '2026-10-18');
    if (location.pathname === '/timers') {
      for (const container of document.querySelectorAll('.timer-time, #day ~ .custom-select-phantom, #day ~ .custom-select-trigger')) {
        const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
        while (walker.nextNode()) walker.currentNode.textContent = normalize(walker.currentNode.textContent);
      }
    }
    for (const footer of document.querySelectorAll('footer p')) {
      const walker = document.createTreeWalker(footer, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        walker.currentNode.textContent = walker.currentNode.textContent.replace(/© \d{4}/, '© 2026');
      }
    }
    if (location.pathname === '/configurations') {
      const prefix = storagePathNormalization.prefix;
      for (const input of document.querySelectorAll('#vdr_video_dir, #vdr_config_dir, #archive_base_dir')) {
        input.value = input.value.replace(prefix, '/fixture');
      }
      const table = document.querySelector('#archive_profiles_table');
      if (table) {
        const walker = document.createTreeWalker(table, NodeFilter.SHOW_TEXT);
        while (walker.nextNode()) {
          walker.currentNode.textContent = walker.currentNode.textContent.replaceAll(prefix, '/fixture');
        }
      }
    }
  }, metadata);
  await expect(page).toHaveScreenshot(`${name}.png`, {
    fullPage: true,
    ...screenshotComparisonOptions[name],
  });
}
