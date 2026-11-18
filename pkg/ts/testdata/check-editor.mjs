import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import path from 'node:path';

const directory = process.argv[2];
const server = spawn(process.execPath, [
  path.join(directory, 'node_modules/typescript/lib/tsserver.js'),
  '--disableAutomaticTypingAcquisition',
], { cwd: directory, stdio: ['pipe', 'pipe', 'inherit'] });
const pending = new Map();
let sequence = 0;
let buffer = Buffer.alloc(0);

function send(command, args) {
  const seq = ++sequence;
  server.stdin.write(JSON.stringify({ seq, type: 'request', command, arguments: args }) + '\n');
  return seq;
}

function request(command, args) {
  return new Promise((resolve, reject) => {
    pending.set(send(command, args), { resolve, reject });
  });
}

server.stdout.on('data', (data) => {
  buffer = Buffer.concat([buffer, data]);
  while (true) {
    const boundary = buffer.indexOf('\r\n\r\n');
    if (boundary < 0) return;
    const length = Number(buffer.subarray(0, boundary).toString().match(/Content-Length: (\d+)/)[1]);
    const start = boundary + 4;
    if (buffer.length < start + length) return;
    const message = JSON.parse(buffer.subarray(start, start + length).toString());
    buffer = buffer.subarray(start + length);
    if (message.type !== 'response') continue;
    const handler = pending.get(message.request_seq);
    if (!handler) continue;
    pending.delete(message.request_seq);
    if (message.success) handler.resolve(message.body);
    else handler.reject(new Error(message.message));
  }
});

server.on('error', (error) => {
  for (const handler of pending.values()) handler.reject(error);
});
server.on('exit', (code) => {
  for (const handler of pending.values()) handler.reject(new Error(`tsserver exited: ${code}`));
});
const timeout = setTimeout(() => server.kill(), 30_000);

try {
  for (const [name, project] of [
    ['deployment.test.ts', 'tsconfig.test.json'],
    ['deployment.ts', 'tsconfig.app.json'],
  ]) {
    const file = path.join(directory, 'src', name);
    send('open', { file });
    const info = await request('projectInfo', { file, needFileNameList: false });
    assert.equal(path.basename(info.configFileName), project, `${name}: ${info.configFileName}`);
    for (const command of ['syntacticDiagnosticsSync', 'semanticDiagnosticsSync']) {
      const diagnostics = await request(command, { file });
      assert.deepEqual(diagnostics, [], `${name}: ${JSON.stringify(diagnostics)}`);
    }
    console.log(`${name}: ${project}, no diagnostics`);
  }
} finally {
  clearTimeout(timeout);
  server.kill();
}
