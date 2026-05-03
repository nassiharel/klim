import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
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

// Try to load enriched github_info from the published marketplace cache.
// Falls back gracefully if unavailable (CI, fresh clone, etc.).
function loadGitHubInfo() {
  const candidates = [
    // Published marketplace.yaml in repo root (CI-assembled)
    path.resolve(import.meta.dirname, '../../marketplace.yaml'),
  ];

  // Local clim cache (user's machine)
  if (process.platform === 'win32') {
    const appdata = process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming');
    candidates.push(path.join(appdata, 'clim', 'marketplace', 'marketplace-cache.yaml'));
  } else if (process.platform === 'darwin') {
    candidates.push(path.join(os.homedir(), 'Library', 'Application Support', 'clim', 'marketplace', 'marketplace-cache.yaml'));
  } else {
    const configDir = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config');
    candidates.push(path.join(configDir, 'clim', 'marketplace', 'marketplace-cache.yaml'));
  }

  for (const p of candidates) {
    if (fs.existsSync(p)) {
      try {
        const data = yaml.load(fs.readFileSync(p, 'utf8'));
        const toolList = data?.tools || data;
        if (Array.isArray(toolList)) {
          const map = {};
          for (const t of toolList) {
            if (t.name && t.github_info) map[t.name] = t.github_info;
          }
          if (Object.keys(map).length > 0) {
            console.log(`  Loaded github_info for ${Object.keys(map).length} tools from ${p}`);
            return map;
          }
        }
      } catch { /* ignore parse errors */ }
    }
  }
  console.log('  No enriched marketplace cache found — github_info will be empty');
  return {};
}

const githubInfoMap = loadGitHubInfo();

const tools = loadYamlDir(TOOLS_DIR).map(t => ({
  name: t.name,
  display_name: t.display_name || t.name,
  category: t.category || 'Other',
  tags: Array.isArray(t.tags) ? t.tags : [],
  binary_names: Array.isArray(t.binary_names) ? t.binary_names : [],
  github: t.github || null,
  packages: t.packages || {},
  github_info: githubInfoMap[t.name] || null,
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
