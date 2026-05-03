import fs from 'node:fs';
import path from 'node:path';
import yaml from 'js-yaml';

const TOOLS_DIR = path.resolve(import.meta.dirname, '../../marketplace/tools');
const PACKS_DIR = path.resolve(import.meta.dirname, '../../marketplace/packs');
const OUT = path.resolve(import.meta.dirname, '../src/data/marketplace.json');

function loadYamlDir(dir) {
  return fs.readdirSync(dir)
    .filter(f => f.endsWith('.yaml') || f.endsWith('.yml'))
    .map(f => yaml.load(fs.readFileSync(path.join(dir, f), 'utf8')))
    .filter(Boolean);
}

const tools = loadYamlDir(TOOLS_DIR).map(t => ({
  name: t.name,
  display_name: t.display_name || t.name,
  category: t.category || 'Other',
  tags: Array.isArray(t.tags) ? t.tags : [],
  github: t.github || null,
  packages: t.packages || {},
}));

const packs = loadYamlDir(PACKS_DIR).map(p => ({
  name: p.name,
  display_name: p.display_name || p.name,
  description: p.description || '',
  icon: p.icon || '📦',
  tools: Array.isArray(p.tools) ? p.tools : [],
}));

tools.sort((a, b) => a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' }));
packs.sort((a, b) => a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' }));

fs.mkdirSync(path.dirname(OUT), { recursive: true });
fs.writeFileSync(OUT, JSON.stringify({ tools, packs }, null, 2));

console.log(`Generated marketplace.json: ${tools.length} tools, ${packs.length} packs`);
