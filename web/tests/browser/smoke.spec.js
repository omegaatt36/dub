const { test, expect } = require('@playwright/test');
const { mkdtemp, mkdir, writeFile, readdir, readFile, rm } = require('node:fs/promises');
const { tmpdir } = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { spawnHost } = require('./host');

let binaryDir;
let binary;
test.beforeAll(async () => {
  binaryDir = await mkdtemp(path.join(tmpdir(), 'dub-browser-bin-'));
  binary = path.join(binaryDir, 'host');
  execFileSync('go', ['build', '-tags', 'browser', '-o', binary, './cmd/browserhost'], {
    cwd: path.resolve(__dirname, '../../..'), stdio: 'inherit',
  });
});
test.afterAll(async () => {
  if (binaryDir) await rm(binaryDir, { recursive: true, force: true });
});

const smoke = test.extend({
  fixture: async ({ page }, use) => {
    const root = await mkdtemp(path.join(tmpdir(), 'dub-browser-files-'));
    let host;
    let initialError;
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    try {
      await mkdir(path.join(root, 'first'));
      await mkdir(path.join(root, 'second'));
      await writeFile(path.join(root, 'first', 'alpha.txt'), 'alpha contents\n');
      await writeFile(path.join(root, 'first', 'beta.txt'), 'beta contents\n');
      await writeFile(path.join(root, 'second', 'other.txt'), 'other contents\n');
      host = spawnHost(binary, ['-root', root, '-web', path.resolve(__dirname, '../..'),
        '-addr', '127.0.0.1:0']);
      const url = await host.ready;
      await page.goto(url);
      // Only the directory picker callback is injected. Opening and every
      // subsequent action use the actual DOM, bridge, HTMX, and HTTP handlers.
      await page.locator('[hx-post="/api/select-directory"]').click();
      await expect(page.getByRole('textbox', { name: 'New name for alpha.txt' })).toBeVisible();
      await use({ root, errors });
      expect(errors).toEqual([]);
    } catch (error) {
      initialError = error;
      throw error;
    } finally {
      let cleanupError;
      try {
        await host?.stop();
      } catch (error) {
        cleanupError = error;
      } finally {
        try {
          await rm(root, { recursive: true, force: true });
        } catch (error) {
          cleanupError ??= error;
        }
      }
      if (initialError || cleanupError || test.info().status !== test.info().expectedStatus) {
        try {
          await test.info().attach('host.log', {
            body: `${host?.logs() ?? ''}\n${cleanupError ?? ''}`, contentType: 'text/plain',
          });
        } catch (error) {
          cleanupError ??= error;
        }
      }
      if (!initialError && cleanupError) throw cleanupError;
    }
  },
});

smoke('manual draft survives an immediate sort and untick by file identity', async ({ page, fixture }) => {
  const alpha = page.getByRole('textbox', { name: 'New name for alpha.txt' });
  await alpha.fill('renamed-alpha');
  // No autosave wait: trigger the competing HTMX action in the same turn.
  await page.locator('#sort-order').selectOption('name:desc');
  await expect(alpha).toHaveValue('renamed-alpha');
  await expect(page.locator('#name-1')).toHaveAttribute('aria-label', 'New name for alpha.txt');
  await page.getByRole('textbox', { name: 'New name for beta.txt' }).fill('renamed-beta');
  await page.getByRole('checkbox', { name: 'Include alpha.txt in the rename' }).uncheck();
  await expect(page.getByRole('textbox', { name: 'New name for beta.txt' })).toHaveValue('renamed-beta');
  await expect(alpha).toHaveCount(0);
  await page.getByRole('checkbox', { name: 'Include alpha.txt in the rename' }).check();
  await expect(alpha).toHaveValue('renamed-alpha');
  expect((await readdir(path.join(fixture.root, 'first'))).sort()).toEqual(['alpha.txt', 'beta.txt']);
});

smoke('pending autosave is flushed before folder navigation and asks before discard', async ({ page, fixture }) => {
  const alpha = page.getByRole('textbox', { name: 'New name for alpha.txt' });
  await alpha.fill('keep-this-draft');
  const openSecond = page.locator('[data-tree-open]').filter({ hasText: 'second' });
  await openSecond.click();
  await expect(page.locator('#discard-prompt')).toBeVisible();
  await expect(alpha).toHaveValue('keep-this-draft');
  await page.getByRole('button', { name: 'Keep editing', exact: true }).click();
  await expect(page.locator('#discard-prompt')).toHaveCount(0);
  await expect(alpha).toHaveValue('keep-this-draft');
  await openSecond.click();
  await page.getByRole('button', { name: 'Discard and open', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'New name for other.txt' })).toBeVisible();
  expect(await readFile(path.join(fixture.root, 'first', 'alpha.txt'), 'utf8')).toBe('alpha contents\n');
});

smoke('desktop callback entry flushes a draft before a dropped-folder request', async ({ page, fixture }) => {
  const alpha = page.getByRole('textbox', { name: 'New name for alpha.txt' });
  await alpha.fill('draft-before-drop');
  // Invoke only the JS entry point used by Wails. This does not simulate an
  // operating-system drag or claim coverage of native event delivery.
  await page.evaluate(paths => window.dubDropFiles({ paths, target: 'scan' }),
    [path.join(fixture.root, 'second')]);
  await expect(page.locator('#discard-prompt')).toBeVisible();
  await expect(alpha).toHaveValue('draft-before-drop');
  await page.getByRole('button', { name: 'Keep editing', exact: true }).click();
  await expect(alpha).toHaveValue('draft-before-drop');
  expect((await readdir(path.join(fixture.root, 'first'))).sort()).toEqual(['alpha.txt', 'beta.txt']);
});

smoke('preview and confirm do not touch disk; execute and undo preserve contents', async ({ page, fixture }) => {
  const dir = path.join(fixture.root, 'first');
  await page.getByRole('textbox', { name: 'New name for alpha.txt' }).fill('renamed-alpha');
  // Manual autosave returns the server-rendered preview without a Preview button.
  await expect(page.getByRole('table', { name: 'Rename preview' })).toBeVisible();
  expect((await readdir(dir)).sort()).toEqual(['alpha.txt', 'beta.txt']);
  await page.locator('#execute-btn').click();
  await expect(page.locator('#execute-confirm')).toBeVisible();
  expect((await readdir(dir)).sort()).toEqual(['alpha.txt', 'beta.txt']);
  await page.locator('#execute-yes').click();
  await expect.poll(async () => (await readdir(dir)).sort()).toEqual(['beta.txt', 'renamed-alpha.txt']);
  expect(await readFile(path.join(dir, 'renamed-alpha.txt'), 'utf8')).toBe('alpha contents\n');
  await page.locator('[hx-post="/api/undo"]').first().click();
  await expect.poll(async () => (await readdir(dir)).sort()).toEqual(['alpha.txt', 'beta.txt']);
  expect(await readFile(path.join(dir, 'alpha.txt'), 'utf8')).toBe('alpha contents\n');
  expect(await readFile(path.join(dir, 'beta.txt'), 'utf8')).toBe('beta contents\n');
});
