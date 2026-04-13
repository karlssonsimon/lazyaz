// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	site: 'https://karlssonsimon.github.io',
	base: '/lazyaz',
	integrations: [
		starlight({
			title: 'lazyaz',
			description:
				'A TUI for inspecting Azure Service Bus, Blob Storage, and Key Vault',
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/karlssonsimon/lazyaz',
				},
			],
			sidebar: [
				{ label: 'Introduction', link: '/' },
				{ label: 'Getting Started', slug: 'getting-started' },
				{ label: 'Navigation', slug: 'navigation' },
				{
					label: 'Resources',
					items: [
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
			],
		}),
	],
});
