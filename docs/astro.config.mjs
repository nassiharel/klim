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
            { label: 'Marketplace', slug: 'guides/marketplace' },
            { label: 'Batch Updates', slug: 'guides/batch-updates' },
            { label: 'Backup & Restore', slug: 'guides/backup-restore' },
            { label: 'Team Manifests', slug: 'guides/team-manifests' },
            { label: 'Dashboard', slug: 'guides/dashboard' },
            { label: 'Adding Tools', slug: 'guides/adding-tools' },
            { label: 'Adding Packs', slug: 'guides/adding-packs' },
          ],
        },
        {
          label: 'CLI Reference',
          items: [
            { label: 'list', slug: 'reference/commands/list' },
            { label: 'export', slug: 'reference/commands/export' },
            { label: 'import', slug: 'reference/commands/import' },
            { label: 'check', slug: 'reference/commands/check' },
            { label: 'update', slug: 'reference/commands/update' },
            { label: 'share', slug: 'reference/commands/share' },
            { label: 'open', slug: 'reference/commands/open' },
            { label: 'config', slug: 'reference/commands/config' },
            { label: 'tools', slug: 'reference/commands/tools' },
            { label: 'version', slug: 'reference/commands/version' },
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
