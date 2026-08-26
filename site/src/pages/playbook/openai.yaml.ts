import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

export const prerender = true;

export async function GET() {
  const configuration = await readFile(
    resolve(process.cwd(), '../playbook/agents/openai.yaml'),
    'utf8',
  );

  return new Response(configuration, {
    headers: { 'Content-Type': 'application/yaml; charset=utf-8' },
  });
}
