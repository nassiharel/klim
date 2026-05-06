import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// `site` and `base` are configurable per deployment target so canonical
// URLs, sitemap entries, and OG meta tags reflect the real public URL:
//
//   - github.io project pages (combined deploy): site=
//     https://<owner>.github.io/<repo> and base=/docs/. The marketing
//     website lives at the root and the docs are mounted under /docs/.
//   - Custom subdomain (docs.klim.dev): site=https://docs.klim.dev
//     and base=/. Mount as the apex of a dedicated host.
//
// The combined Pages workflow (.github/workflows/deploy-pages.yml)
// copies the docs build into `_pages/docs/` of the unified Pages
// artifact, so base=/docs/ is the correct default for that workflow.
// Both env vars are wired from the workflow so a future custom-domain
// switch only needs a workflow edit, not a code change here.
const docsBase = process.env.KLIM_DOCS_BASE ?? '/docs/';
const docsSite = process.env.KLIM_DOCS_SITE ?? 'https://docs.klim.dev';

export default defineConfig({
  site: docsSite,
  base: docsBase,
  integrations: [
    starlight({
      title: 'klim',
      description: 'Documentation for klim — deterministic developer toolchains across local machines, teams, CI, and agents',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/nassiharel/klim' },
      ],
      editLink: {
        baseUrl: 'https://github.com/nassiharel/klim/edit/main/docs/',
      },
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Quick Start', slug: 'getting-started/quickstart' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'TUI Overview', slug: 'guides/tui-overview' },
            { label: 'Favorites', slug: 'guides/favorites' },
            { label: 'Batch Updates', slug: 'guides/batch-updates' },
            { label: 'Backup & Restore', slug: 'guides/backup-restore' },
            { label: 'Team Manifests', slug: 'guides/team-manifests' },
            { label: 'Dashboard', slug: 'guides/dashboard' },
            { label: 'Security', slug: 'guides/security' },
            { label: 'Shell Integration', slug: 'guides/shell-integration' },
            { label: 'Environment Diff', slug: 'guides/environment-diff' },
            { label: 'Adding Tools', slug: 'guides/adding-tools' },
            { label: 'Adding Packs', slug: 'guides/adding-packs' },
          ],
        },
        {
          label: 'CLI Reference',
          items: [
            {
              label: 'Core',
              items: [
                { label: 'list', slug: 'reference/commands/list' },
                { label: 'browser', slug: 'reference/commands/browser' },
                { label: 'update', slug: 'reference/commands/update' },
                { label: 'version', slug: 'reference/commands/version' },
              ],
            },
            {
              label: 'Project',
              items: [
                { label: 'check', slug: 'reference/commands/check' },
                { label: 'init', slug: 'reference/commands/init' },
                { label: 'generate', slug: 'reference/commands/generate' },
              ],
            },
            {
              label: 'Tools',
              items: [
                { label: 'info', slug: 'reference/commands/info' },
                { label: 'search', slug: 'reference/commands/search' },
                { label: 'install', slug: 'reference/commands/install' },
                { label: 'upgrade', slug: 'reference/commands/upgrade' },
                { label: 'remove', slug: 'reference/commands/remove' },
                { label: 'diff', slug: 'reference/commands/diff' },
                { label: 'onboard', slug: 'reference/commands/onboard' },
                { label: 'try', slug: 'reference/commands/try' },
                { label: 'watch', slug: 'reference/commands/watch' },
                { label: 'why', slug: 'reference/commands/why' },
              ],
            },
            {
              label: 'History',
              items: [
                { label: 'trail', slug: 'reference/commands/trail' },
              ],
            },
            {
              label: 'Backup & Sharing',
              items: [
                { label: 'export', slug: 'reference/commands/export' },
                { label: 'import', slug: 'reference/commands/import' },
                { label: 'share', slug: 'reference/commands/share' },
                { label: 'env', slug: 'reference/commands/env' },
              ],
            },
            {
              label: 'Health & Security',
              items: [
                { label: 'security', slug: 'reference/commands/security' },
                { label: 'security vuln', slug: 'reference/commands/vuln' },
                { label: 'score', slug: 'reference/commands/score' },
              ],
            },
            {
              label: 'Shell Integration',
              items: [
                { label: 'shell completion', slug: 'reference/commands/completion' },
                { label: 'shell hook', slug: 'reference/commands/hook' },
                { label: 'proxy', slug: 'reference/commands/proxy' },
              ],
            },
            {
              label: 'Configuration',
              items: [
                { label: 'config', slug: 'reference/commands/config' },
                { label: 'config marketplace', slug: 'reference/commands/marketplace' },
              ],
            },
          ],
        },
        {
          label: 'Configuration',
          items: [
            { label: 'config.yaml Reference', slug: 'reference/configuration' },
          ],
        },
        {
          label: 'Contributing',
          items: [
            { label: 'Development', slug: 'contributing/development' },
            { label: 'Architecture', slug: 'contributing/architecture' },
          ],
        },
      ],
    }),
  ],
});
