import { defineConfig } from 'vitepress'

// c3x docs site config.
//
// Lives at c3x.dev/docs. VitePress for the static-generator pick
// because the c3x team is small and we want zero JS to ship to
// readers — VitePress emits plain Markdown→HTML with the Vue
// runtime gated behind a small interactive shell only when needed.
//
// The sidebar mirrors the user's mental model: install → estimate
// → CI integration → reference. Everything reachable in two
// clicks.
export default defineConfig({
  title: 'c3x',
  description: 'Cloud cost estimation for Terraform & CloudFormation. Open source, no API key, no SaaS.',
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg' }],
    ['meta', { name: 'theme-color', content: '#1e88e5' }],
  ],

  themeConfig: {
    siteTitle: 'c3x',
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'Integrations', link: '/integrations/' },
      { text: 'Reference', link: '/reference/cli' },
      { text: 'Catalog', link: '/reference/catalog' },
      {
        text: 'GitHub',
        link: 'https://github.com/c3xdev/c3x',
      },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Getting started',
          items: [
            { text: 'Install', link: '/guide/getting-started' },
            { text: 'Your first estimate', link: '/guide/first-estimate' },
            { text: 'Usage file', link: '/guide/usage-file' },
            { text: 'What-if overrides', link: '/guide/what-if' },
            { text: 'Self-hosting the pricing API', link: '/guide/self-hosted' },
          ],
        },
        {
          text: 'Concepts',
          items: [
            { text: 'How c3x prices resources', link: '/guide/how-pricing-works' },
            { text: 'STATIC vs LIVE vs FREE', link: '/guide/resource-status' },
            { text: 'Recommendations', link: '/guide/recommendations' },
            { text: 'Policy gates', link: '/guide/policy' },
          ],
        },
      ],
      '/integrations/': [
        {
          text: 'CI / Forge integration',
          items: [
            { text: 'Overview', link: '/integrations/' },
            { text: 'GitHub Actions', link: '/integrations/github' },
            { text: 'GitLab CI', link: '/integrations/gitlab' },
            { text: 'Bitbucket Pipelines', link: '/integrations/bitbucket' },
            { text: 'Azure DevOps', link: '/integrations/azuredevops' },
            { text: 'Atlantis', link: '/integrations/atlantis' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'CLI command map', link: '/reference/cli' },
            { text: 'Output formats', link: '/reference/formats' },
            { text: 'Supported resources', link: '/reference/catalog' },
            { text: 'Architecture', link: '/reference/architecture' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/c3xdev/c3x' },
    ],
    search: {
      provider: 'local',
    },
    editLink: {
      pattern: 'https://github.com/c3xdev/c3x/edit/main/docs-site/:path',
    },
    footer: {
      message: 'Apache-2.0 licensed.',
      copyright: 'Copyright © 2026 c3x contributors.',
    },
  },
})
