import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';

// `site` and `base` are configurable per deployment target so canonical
// URLs, sitemap entries, and OG meta tags reflect the real public URL,
// and internal asset paths resolve correctly under sub-paths:
//
//   - github.io project pages: site=https://<owner>.github.io and
//     base=/<repo>/. The workflow wires both via env vars so the
//     defaults below remain pointed at the eventual custom-domain
//     target without forcing a config edit at switch-over time.
//   - Custom domain (klim.dev): site=https://klim.dev and base=/.
//     Add a CNAME file to website/public/ to take effect.
const websiteBase = process.env.KLIM_WEBSITE_BASE ?? '/';
const websiteSite = process.env.KLIM_WEBSITE_SITE ?? 'https://klim.dev';

export default defineConfig({
  site: websiteSite,
  base: websiteBase,
  vite: {
    plugins: [tailwindcss()],
  },
});
