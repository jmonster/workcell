import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

export const prerender = true;

export async function GET() {
  const skill = await readFile(resolve(process.cwd(), '../playbook/SKILL.md'), 'utf8');

  return new Response(skill, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
}
