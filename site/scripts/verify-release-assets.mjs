import { stat } from 'node:fs/promises';
import path from 'node:path';

const siteRoot = path.resolve(import.meta.dirname, '..');
const publicRoot = path.join(siteRoot, 'public');
const downloadsRoot = path.join(publicRoot, 'downloads');
const binaries = [
  'workcell-darwin-arm64',
  'workcell-darwin-amd64',
  'workcell-linux-arm64',
  'workcell-linux-amd64',
];

for (const binary of binaries) {
  const info = await stat(path.join(downloadsRoot, binary));
  if (!info.isFile() || info.size === 0 || (info.mode & 0o111) === 0) {
    throw new Error(`release binary is missing, empty, or not executable: ${binary}`);
  }
}

for (const script of ['install.sh', 'demo.sh']) {
  const info = await stat(path.join(publicRoot, script));
  if (!info.isFile() || (info.mode & 0o111) === 0) {
    throw new Error(`release script is missing or not executable: ${script}`);
  }
}

console.log(`verified_release_binaries=${binaries.length}`);
