<script lang="ts">
	type Route = 'live' | 'congress' | 'ipo';
	let { active }: { active: Route } = $props();

	const links: { href: string; label: string; key: Route }[] = [
		{ href: '/', label: 'Live', key: 'live' },
		{ href: '/congress', label: 'Congress', key: 'congress' },
		{ href: '/ipo', label: 'IPOs', key: 'ipo' }
	];
</script>

<header class="sticky top-0 z-40 border-b border-[#1c1c1c] bg-[#0a0a0a]/90 backdrop-blur">
	<div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
		<a href="/" class="flex items-center gap-2">
			<span class="h-2.5 w-2.5 rounded-full bg-[#00c805]"></span>
			<span class="text-lg font-bold tracking-tight text-white">Watchtower</span>
		</a>
		<nav class="flex items-center gap-1 text-sm">
			{#each links as link (link.key)}
				<a
					href={link.href}
					class="rounded-md px-3 py-1.5 font-medium transition-colors
						{active === link.key
						? 'text-white'
						: 'text-[#9a9a9a] hover:text-white'}"
				>
					{link.label}
				</a>
			{/each}
		</nav>
	</div>
</header>
