// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';

// https://astro.build/config
export default defineConfig({
	site: 'https://karlssonsimon.github.io',
	base: '/lazyaz',
	integrations: [
		sitemap(),
		starlight({
			title: 'lazyaz',
			description:
				'A fast, keyboard-driven terminal UI for Azure. Like lazygit, but for Azure.',
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/karlssonsimon/lazyaz',
				},
			],
			head: [
				// Open Graph
				{
					tag: 'meta',
					attrs: {
						property: 'og:type',
						content: 'website',
					},
				},
				{
					tag: 'meta',
					attrs: {
						property: 'og:site_name',
						content: 'lazyaz',
					},
				},
				// Additional SEO
				{
					tag: 'meta',
					attrs: {
						name: 'keywords',
						content:
							'azure, terminal, tui, cli, lazygit, azure portal alternative, azure terminal tool, keyboard-driven',
					},
				},
			],
			sidebar: [
				{ label: 'Introduction', link: '/' },
				{ label: 'Getting Started', slug: 'getting-started' },
				{ label: 'Why lazyaz', slug: 'why-lazyaz' },
				{ label: 'Navigation', slug: 'navigation' },
				{
					label: 'Resources',
					items: [
						{ label: 'Dashboard', slug: 'resources/dashboard' },
						{ label: 'Blob Storage', slug: 'resources/blob-storage' },
						{
							label: 'Service Bus',
							slug: 'resources/service-bus',
						},
						{ label: 'Key Vault', slug: 'resources/key-vault' },
					],
				},
				{
					label: 'Configuration',
					items: [
						{ label: 'Overview', slug: 'configuration/overview' },
						{ label: 'Keymaps', slug: 'configuration/keymaps' },
					],
				},
				{ label: 'Authentication', slug: 'authentication' },
				{ label: 'Multi-Tenant Support', slug: 'multi-tenant' },
				{ label: 'FAQ & Troubleshooting', slug: 'faq' },
			],
		}),
	],
});
