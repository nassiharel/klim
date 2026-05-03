import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.clim.dev',
  integrations: [
    starlight({
      title: 'clim',
      description: 'Documentation for clim — the cross-platform developer tool manager',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: false,
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/nassiharel/clim' },
      ],
      editLink: {
        baseUrl: 'https://github.com/nassiharel/clim/edit/main/docs/',
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
            { label: 'Doctor & Audit', slug: 'guides/doctor-audit' },
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
              ],
            },
            {
              label: 'Health & Security',
              items: [
                { label: 'doctor', slug: 'reference/commands/doctor' },
                { label: 'audit', slug: 'reference/commands/audit' },
                { label: 'compliance', slug: 'reference/commands/compliance' },
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
