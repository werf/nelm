import { writeFile } from 'node:fs/promises';
import { parseArgs } from 'node:util';
import { describe } from 'cjs-dependency';

const { values } = parseArgs({
  args: process.argv.slice(2),
  options: {
    'input-file': { type: 'string' },
    'output-file': { type: 'string' },
  },
});

await writeFile(values['output-file']!, JSON.stringify(describe()));
