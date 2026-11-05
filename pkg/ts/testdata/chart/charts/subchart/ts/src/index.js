import { writeFile } from 'node:fs/promises';
import { parseArgs } from 'node:util';

const { values } = parseArgs({
  args: process.argv.slice(2),
  options: {
    'input-file': { type: 'string' },
    'output-file': { type: 'string' },
  },
});

await writeFile(values['output-file'], JSON.stringify({ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'subchart' } }));
