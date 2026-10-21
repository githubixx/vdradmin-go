import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { mkdtemp, mkdir, copyFile, writeFile, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const exec = promisify(execFile);
const digest = body => createHash('sha256').update(body).digest('hex');

async function setup(t, body, contentType) {
  const root = await mkdtemp(path.join(tmpdir(), 'vdr-e2e-resources-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const dir = path.join(root, 'test/e2e');
  const cache = path.join(root, '.cache/e2e-resources');
  await mkdir(path.join(dir, 'resources'), { recursive: true });
  await mkdir(cache, { recursive: true });
  await copyFile(new URL('./resources.mjs', import.meta.url), path.join(dir, 'resources.mjs'));
  const sha = digest(body);
  await writeFile(path.join(dir, 'resources.lock.json'), JSON.stringify({
    'https://resources.invalid/resource': { sha256: sha, contentType },
  }));
  return {
    root, dir, cache, sha,
    run: (args = [], options = {}) => exec(process.execPath, [...args, path.join(dir, 'resources.mjs')], options),
  };
}

test('cold cache uses exact locked CSS without network access', async t => {
  const body = 'body { font-family: Test; }\n';
  const fixture = await setup(t, body, 'text/css; charset=utf-8');
  await writeFile(path.join(fixture.dir, 'resources', `${fixture.sha}.css`), body);
  const { stdout } = await fixture.run();
  assert.match(stdout, /Verified 1 locked browser resources/);
  assert.equal(await readFile(path.join(fixture.cache, fixture.sha), 'utf8'), body);
});

test('missing CSS fixture fails even with a valid warm cache', async t => {
  const body = 'body {}\n';
  const fixture = await setup(t, body, 'text/css');
  await writeFile(path.join(fixture.cache, fixture.sha), body);
  await assert.rejects(fixture.run(), error => {
    assert.match(error.stderr, /ENOENT/);
    assert.match(error.stderr, /resources.*\.css/);
    return true;
  });
});

test('modified CSS fixture fails instead of using the warm cache', async t => {
  const body = 'body {}\n';
  const fixture = await setup(t, body, 'text/css');
  await writeFile(path.join(fixture.cache, fixture.sha), body);
  await writeFile(path.join(fixture.dir, 'resources', `${fixture.sha}.css`), 'changed');
  await assert.rejects(fixture.run(), error => {
    assert.match(error.stderr, /Resource changed:/);
    return true;
  });
});

test('verified cached binary resources need no network access', async t => {
  const body = Buffer.from([0, 1, 2, 255]);
  const fixture = await setup(t, body, 'font/woff2');
  await writeFile(path.join(fixture.cache, fixture.sha), body);
  await fixture.run();
  assert.deepEqual(await readFile(path.join(fixture.cache, fixture.sha)), body);
});

test('modified binary resources are rejected', async t => {
  const fixture = await setup(t, 'original', 'font/woff2');
  await writeFile(path.join(fixture.cache, fixture.sha), 'changed');
  await assert.rejects(fixture.run(), error => {
    assert.match(error.stderr, /Resource changed:/);
    return true;
  });
});

test('cold binary downloads are verified before caching', async t => {
  const fixture = await setup(t, 'downloaded', 'font/woff2');
  const mock = path.join(fixture.dir, 'fetch.mjs');
  await writeFile(mock, "globalThis.fetch = async () => new Response('changed');\n");
  await assert.rejects(fixture.run(['--import', mock]), error => {
    assert.match(error.stderr, /Resource changed:/);
    return true;
  });
  await assert.rejects(readFile(path.join(fixture.cache, fixture.sha)), { code: 'ENOENT' });
  await writeFile(mock, "globalThis.fetch = async () => new Response('downloaded');\n");
  await fixture.run(['--import', mock]);
  assert.equal(await readFile(path.join(fixture.cache, fixture.sha), 'utf8'), 'downloaded');
});

test('lock updates discover Chroma, application and override resources and save CSS', async t => {
  const fixture = await setup(t, '', 'text/css');
  for (const dir of ['web/static/css', 'web/templates', 'web/themes/custom', 'chroma/assets/css', 'chroma/assets/themes/example', 'bin']) {
    await mkdir(path.join(fixture.root, dir), { recursive: true });
  }
  for (const [file, name] of [
    ['web/static/css/base.css', 'application'],
    ['web/themes/custom/theme.css', 'override'],
    ['chroma/assets/css/base.css', 'shared'],
    ['chroma/assets/themes/example/theme.css', 'theme'],
  ]) {
    await writeFile(path.join(fixture.root, file), `@import url('https://resources.invalid/${name}');`);
  }
  await writeFile(path.join(fixture.root, 'web/templates/base.html'), '<script src="https://resources.invalid/script"></script>');
  await writeFile(path.join(fixture.root, 'bin/go'),
    `#!/usr/bin/env node
import assert from 'node:assert/strict';
assert.equal(process.env.GOWORK, 'off');
assert.deepEqual(process.argv.slice(2), [
  'list', '-mod=readonly', '-m', '-f', '{{.Dir}}', 'github.com/githubixx/chroma-ui',
]);
console.log(process.env.TEST_CHROMA_DIR);
`, { mode: 0o755 });
  const mock = path.join(fixture.dir, 'fetch.mjs');
  await writeFile(mock, `
globalThis.fetch = async url => new Response(
  url.endsWith('/font') ? 'font bytes' : '/* ' + url + ' */ url(https://resources.invalid/font)',
  { headers: { 'content-type': url.endsWith('/font') ? 'font/woff2' : 'text/css' } },
);
`);
  await exec(process.execPath, ['--import', mock, path.join(fixture.dir, 'resources.mjs'), '--update-lock'], {
    env: {
      ...process.env,
      PATH: `${path.join(fixture.root, 'bin')}${path.delimiter}${process.env.PATH}`,
      TEST_CHROMA_DIR: path.join(fixture.root, 'chroma'),
    },
  });
  const lock = JSON.parse(await readFile(path.join(fixture.dir, 'resources.lock.json'), 'utf8'));
  assert.deepEqual(Object.keys(lock).sort(), ['application', 'font', 'override', 'script', 'shared', 'theme']
    .map(name => `https://resources.invalid/${name}`).sort());
  for (const resource of Object.values(lock)) {
    if (resource.contentType !== 'text/css') continue;
    const css = await readFile(path.join(fixture.dir, 'resources', `${resource.sha256}.css`));
    assert.equal(digest(css), resource.sha256);
  }
});
