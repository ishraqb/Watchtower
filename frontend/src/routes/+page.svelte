<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { connect, disconnect, latestTick, connectionStatus, prices, type Tick } from '$lib/ws';
	import { createSymbolChart, pushPrice, type SymbolChart } from '$lib/charts';

	const SYMBOLS = ['AAPL', 'TSLA', 'RIVN'];

	let containers: Record<string, HTMLDivElement> = {};
	const charts: Record<string, SymbolChart> = {};

	let status = $state<'connecting' | 'open' | 'closed'>('closed');
	let latestPrices = $state<Record<string, Tick>>({});

	const unsubStatus = connectionStatus.subscribe((s) => (status = s));
	const unsubPrices = prices.subscribe((p) => (latestPrices = p));

	const unsubTick = latestTick.subscribe((tick) => {
		if (!tick) return;
		const sc = charts[tick.symbol];
		if (sc) pushPrice(sc, tick.time, tick.price);
	});

	onMount(() => {
		for (const sym of SYMBOLS) {
			if (containers[sym]) {
				charts[sym] = createSymbolChart(containers[sym]);
			}
		}
		connect();
	});

	onDestroy(() => {
		unsubStatus();
		unsubPrices();
		unsubTick();
		for (const sym of SYMBOLS) charts[sym]?.chart.remove();
		disconnect();
	});

	const statusColor = $derived(
		status === 'open' ? 'bg-emerald-500' : status === 'connecting' ? 'bg-amber-500' : 'bg-rose-500'
	);
</script>

<svelte:head>
	<title>Watchtower — Live Market Dashboard</title>
</svelte:head>

<div class="min-h-screen bg-slate-950 text-slate-100">
	<header class="border-b border-slate-800 px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-bold tracking-tight">Watchtower</h1>
				<p class="text-sm text-slate-400">Real-time market intelligence</p>
			</div>
			<div class="flex items-center gap-3">
				<nav class="flex gap-4 text-sm">
					<a class="text-sky-400 hover:text-sky-300" href="/">Live</a>
					<a class="text-slate-400 hover:text-slate-200" href="/congress">Congress</a>
					<a class="text-slate-400 hover:text-slate-200" href="/ipo">IPOs</a>
				</nav>
				<span class="flex items-center gap-2 rounded-full bg-slate-900 px-3 py-1 text-xs">
					<span class="h-2 w-2 rounded-full {statusColor}"></span>
					{status}
				</span>
			</div>
		</div>
	</header>

	<main class="p-6">
		<div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
			{#each SYMBOLS as sym (sym)}
				<section class="rounded-xl border border-slate-800 bg-slate-900/50 p-4">
					<div class="mb-3 flex items-baseline justify-between">
						<h2 class="text-lg font-semibold">{sym}</h2>
						<span class="font-mono text-xl text-sky-400">
							{latestPrices[sym] ? `$${latestPrices[sym].price.toFixed(2)}` : '—'}
						</span>
					</div>
					<div bind:this={containers[sym]} class="h-64 w-full"></div>
				</section>
			{/each}
		</div>

		{#if status !== 'open'}
			<p class="mt-6 text-center text-sm text-slate-500">
				Waiting for the backend WebSocket at <code>ws://localhost:8080/ws</code>. Markets only stream
				ticks during US trading hours.
			</p>
		{/if}
	</main>
</div>
