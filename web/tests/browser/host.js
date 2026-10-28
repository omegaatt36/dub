const { spawn } = require('node:child_process');

function deadline(promise, milliseconds, message) {
  let timer;
  return Promise.race([
    promise,
    new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(message)), milliseconds); }),
  ]).finally(() => clearTimeout(timer));
}

// Register all lifecycle listeners in the same turn as spawn, before any await.
function spawnHost(command, args, { startupTimeout = 15000, stopTimeout = 2000 } = {}) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'] });
  let logs = '';
  let failure;
  let finish;
  const ended = new Promise(resolve => { finish = resolve; });
  const fail = error => { failure = error; finish(error); };
  child.once('error', error => fail(new Error(`browser host spawn failed: ${error.message}`)));
  child.once('exit', (code, signal) => fail(new Error(`browser host exited (code ${code}, signal ${signal})`)));
  child.stderr.on('data', data => { logs += data; });
  let stdout = '';
  const address = new Promise((resolve, reject) => {
    child.stdout.setEncoding('utf8');
    child.stdout.on('data', data => {
      logs += data;
      stdout += data;
      const newline = stdout.indexOf('\n');
      if (newline < 0) return;
      const url = stdout.slice(0, newline);
      if (!/^http:\/\/127\.0\.0\.1:[1-9][0-9]*$/.test(url)) {
        reject(new Error(`invalid browser host address: ${url}`));
      } else {
        resolve(url);
      }
    });
  });
  const ready = deadline(Promise.race([
    address.then(url => {
      if (failure) throw failure;
      return url;
    }),
    ended.then(error => { throw error; }),
  ]), startupTimeout, 'browser host startup timed out');
  // A caller can perform fixture setup before awaiting ready.
  ready.catch(() => {});
  return {
    ready,
    logs: () => logs,
    async stop() {
      if (failure) return;
      child.kill('SIGTERM');
      try {
        await deadline(ended, stopTimeout, 'browser host termination timed out');
      } catch {
        child.kill('SIGKILL');
        await deadline(ended, stopTimeout, 'browser host SIGKILL timed out');
      }
    },
  };
}

module.exports = { spawnHost };
