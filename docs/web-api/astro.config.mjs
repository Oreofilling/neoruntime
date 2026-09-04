import { defineConfig } from 'astro/config'
import starlight from '@astrojs/starlight'

// Deployed at https://<host>/neoruntime/ (GitHub Pages project site).
const SITE = 'https://camthink-ai.github.io'
const BASE = '/neoruntime'

export default defineConfig({
  site: SITE,
  base: BASE,
  integrations: [
    starlight({
      title: {
        en: 'NeoRuntime Docs',
        'zh-CN': 'NeoRuntime 文档',
      },
      description:
        'NeoRuntime documentation — project wiki plus the bilingual Web API reference for the edge AI computing platform.',
      locales: {
        root: { label: 'English', lang: 'en' },
        zh: { label: '简体中文', lang: 'zh-CN' },
      },
      defaultLocale: 'root',
      lastUpdated: true,
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/camthink-ai/neoruntime' },
      ],
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'Web API',
          translations: { 'zh-CN': 'Web API' },
          collapsed: false,
          items: [
            { slug: 'introduction' },
            { slug: 'quickstart' },
            { slug: 'authentication' },
            { slug: 'errors' },
            { slug: 'update-mechanism' },
            // External full-page Redoc reference. In the Chinese locale the
            // sidebar link resolves to /zh/api-reference/, which public/
            // redirects to the Chinese reference page.
            {
              label: 'API Reference (full page)',
              translations: { 'zh-CN': 'API 参考（全屏页）' },
              link: '/api-reference/',
            },
          ],
        },
        // Project wiki, synced from docs/** at build time by
        // scripts/sync_project_docs.py. Bare autogenerate expands to one
        // group per directory in English and to nothing in Chinese (the
        // wiki is English source material; the zh home page links to it).
        { autogenerate: { directory: 'project', collapsed: true } },
      ],
    }),
  ],
})
