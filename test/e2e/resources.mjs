import { createHash } from 'node:crypto';
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '../..');
const lockPath = path.join(import.meta.dirname, 'resources.lock.json');
const cacheDir = path.join(root, '.cache/e2e-resources');
const update = process.argv.includes('--update-lock');
const digest = body => createHash('sha256').update(body).digest('hex');
const userAgent = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

await mkdir(cacheDir, { recursive: true });

if (!update) {
  const resources = JSON.parse(await readFile(lockPath, 'utf8'));
  for (const [url, resource] of Object.entries(resources)) {
    const target = path.join(cacheDir, resource.sha256);
    let body;
    try {
      body = await readFile(target);
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      body = await download(url);
    }
    if (digest(body) !== resource.sha256) {
      throw new Error(`Resource changed: ${url}. Review before explicitly updating the resource lock.`);
    }
    await writeFile(target, body);
  }
  console.log(`Verified ${Object.keys(resources).length} locked browser resources.`);
} else {
  const cssFiles = [path.join(root, 'web/static/css/base.css')];
  for (const entry of await readdir(path.join(root, 'web/themes'), { withFileTypes: true })) {
    if (entry.isDirectory()) cssFiles.push(path.join(root, 'web/themes', entry.name, 'theme.css'));
  }
  const queue = new Set();
  for (const name of await readdir(path.join(root, 'web/templates'))) {
    if (!name.endsWith('.html')) continue;
    const template = await readFile(path.join(root, 'web/templates', name), 'utf8');
    for (const match of template.matchAll(/<script\b[^>]*\bsrc=["'](https:\/\/[^"']+)["']/g)) {
      queue.add(new URL(match[1]).href);
    }
  }
  for (const file of cssFiles) {
    const css = await readFile(file, 'utf8');
    for (const match of css.matchAll(/@import\s+url\(['"]?(https:\/\/[^'")]+)['"]?\)/g)) {
      queue.add(new URL(match[1]).href);
    }
  }
  const resources = {};
  for (const url of queue) {
    const response = await fetch(url, {
      headers: { 'User-Agent': userAgent },
      signal: AbortSignal.timeout(30_000),
    });
    if (!response.ok) throw new Error(`${response.status}: ${url}`);
    const body = Buffer.from(await response.arrayBuffer());
    const contentType = response.headers.get('content-type');
    if (!contentType) throw new Error(`Missing content type: ${url}`);
    const sha256 = digest(body);
    resources[url] = { sha256, contentType };
    await writeFile(path.join(cacheDir, sha256), body);
    if (contentType.includes('text/css')) {
      for (const match of body.toString().matchAll(/url\(['"]?(https:\/\/[^'")]+)['"]?\)/g)) {
        queue.add(new URL(match[1]).href);
      }
    }
  }
  await writeFile(lockPath, `${JSON.stringify(resources, null, 2)}\n`);
  console.log(`Locked ${Object.keys(resources).length} browser resources. Review resources.lock.json.`);
}

async function download(url) {
  const response = await fetch(url, {
    headers: { 'User-Agent': userAgent },
    signal: AbortSignal.timeout(30_000),
  });
  if (!response.ok) throw new Error(`${response.status}: ${url}`);
  return Buffer.from(await response.arrayBuffer());
}
