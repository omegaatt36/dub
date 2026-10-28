const { test } = require('node:test');
const assert = require('node:assert/strict');
const { spawnHost } = require('./host');

test('spawn failure rejects startup and teardown handles an already-dead child', { timeout: 3000 }, async () => {
  const host = spawnHost('/nonexistent/dub-browser-host', []);
  await assert.rejects(host.ready, /spawn failed/);
  await host.stop();
});

test('early exit reports its cause and captures stderr', { timeout: 3000 }, async () => {
  const host = spawnHost(process.execPath, ['-e', "console.error('bind failed'); process.exit(23)"]);
  await assert.rejects(host.ready, /exited \(code 23/);
  await host.stop();
  assert.match(host.logs(), /bind failed/);
});

test('startup timeout is bounded and the child is stopped', { timeout: 3000 }, async () => {
  const host = spawnHost(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { startupTimeout: 100 });
  await assert.rejects(host.ready, /startup timed out/);
  await host.stop();
});

test('reads a chunked address and escalates an ignored SIGTERM', { timeout: 3000 }, async () => {
  const host = spawnHost(process.execPath, ['-e', `
    process.on('SIGTERM', () => {});
    process.stdout.write('http://127.0.0.1:');
    setTimeout(() => process.stdout.write('54321\\n'), 10);
    setInterval(() => {}, 1000);
  `], { stopTimeout: 100 });
  assert.equal(await host.ready, 'http://127.0.0.1:54321');
  await host.stop();
  await host.stop();
});
