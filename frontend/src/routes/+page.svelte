<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		connect,
		disconnect,
		latestTick,
		connectionStatus,
		prices,
		anomalies,
		type Tick,
		type Anomaly
	} from '$lib/ws';
	import { createSymbolChart, pushPrice, type SymbolChart } from '$lib/charts';
	import { getQuote, type Quote } from '$lib/api';

	const SYMBOLS = ['AAPL', 'TSLA', 'RIVN'];

	let containers: Record<string, HTMLDivElement> = {};
	const charts: Record<string, SymbolChart> = {};

	let status = $state<'connecting' | 'open' | 'closed'>('closed');
	let latestPrices = $state<Record<string, Tick>>({});
	let anomalyList = $state<Anomaly[]>([]);
	let quotes = $state<Record<string, Quote>>({});

	// Whether any live ticks have been received this session.
	const hasLiveTicks = $derived(Object.keys(latestPrices).length > 0);

	// The price shown per symbol: live tick if available, otherwise last-known quote.
	function displayPrice(sym: string): number | undefined {
		if (latestPrices[sym]) return latestPrices[sym].price;
		if (quotes[sym]) return quotes[sym].current;
		return undefined;
	}

	function changeColor(change: number | undefined): string {
		if (change === undefined || change === 0) return 'text-slate-400';
		return change > 0 ? 'text-emerald-400' : 'text-rose-400';
	}

	const unsubStatus = connectionStatus.subscribe((s) => (status = s));
	const unsubPrices = prices.subscribe((p) => (latestPrices = p));
	const unsubAnomalies = anomalies.subscribe((a) => (anomalyList = a));

	const unsubTick = latestTick.subscribe((tick) => {
		if (!tick) return;
		const sc = charts[tick.symbol];
		if (sc) pushPrice(sc, tick.time, tick.price);
	});

	function sentimentColor(score: number | undefined): string {
		if (score === undefined) return 'text-slate-400';
		if (score > 0.05) return 'text-emerald-400';
		if (score < -0.05) return 'text-rose-400';
		return 'text-amber-400';
	}

	function sentimentLabel(score: number | undefined): string {
		if (score === undefined) return 'analyzing…';
		if (score > 0.05) return 'Positive';
		if (score < -0.05) return 'Negative';
		return 'Neutral';
	}

	function fmtTime(ms: number): string {
		return new Date(ms).toLocaleTimeString('en-US');
	}

	onMount(() => {
		for (const sym of SYMBOLS) {
			if (containers[sym]) {
				charts[sym] = createSymbolChart(containers[sym]);
			}
		}
		// Seed each chart with the last-known quote so the dashboard is never
		// blank when markets are closed. Live ticks (if any) continue from here.
		for (const sym of SYMBOLS) {
			getQuote(sym)
				.then((q) => {
					quotes[sym] = q;
					const sc = charts[sym];
					if (sc && q.open > 0 && q.current > 0) {
						const now = Math.floor(Date.now() / 1000);
						// Two-point baseline: today's open → last price.
						pushPrice(sc, (now - 3600) * 1000, q.open);
						pushPrice(sc, now * 1000, q.current);
					}
				})
				.catch(() => {});
		}
		connect();
	});

	onDestroy(() => {
		unsubStatus();
		unsubPrices();
		unsubTick();
		unsubAnomalies();
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

	<main class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[1fr_22rem]">
		<div>
			<div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
				{#each SYMBOLS as sym (sym)}
					<section class="rounded-xl border border-slate-800 bg-slate-900/50 p-4">
						<div class="mb-2 flex items-baseline justify-between">
							<h2 class="text-lg font-semibold">{sym}</h2>
							<div class="text-right">
								<span class="font-mono text-xl text-sky-400">
									{displayPrice(sym) !== undefined
										? `$${displayPrice(sym)!.toFixed(2)}`
										: '—'}
								</span>
								{#if quotes[sym]}
									<span class="ml-1 font-mono text-xs {changeColor(quotes[sym].change)}">
										{quotes[sym].change >= 0 ? '+' : ''}{quotes[sym].change.toFixed(2)}
										({quotes[sym].percent_change.toFixed(2)}%)
									</span>
								{/if}
							</div>
						</div>

						{#if quotes[sym]}
							<div class="mb-3 grid grid-cols-4 gap-1 text-center text-[11px] text-slate-400">
								<div>
									<div class="text-slate-500">Open</div>
									<div class="font-mono text-slate-200">{quotes[sym].open.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-slate-500">High</div>
									<div class="font-mono text-slate-200">{quotes[sym].high.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-slate-500">Low</div>
									<div class="font-mono text-slate-200">{quotes[sym].low.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-slate-500">Prev</div>
									<div class="font-mono text-slate-200">{quotes[sym].previous_close.toFixed(2)}</div>
								</div>
							</div>
						{/if}

						<div bind:this={containers[sym]} class="h-64 w-full"></div>

						{#if !latestPrices[sym] && quotes[sym]}
							<p class="mt-2 text-center text-[11px] text-slate-500">
								Last known price · not currently streaming
							</p>
						{/if}
					</section>
				{/each}
			</div>

			{#if !hasLiveTicks}
				<p class="mt-6 text-center text-sm text-slate-500">
					No live ticks right now — showing each symbol's last-known price and day stats from the
					most recent session. Live charts resume automatically during US trading hours
					(9:30 AM–4:00 PM ET).
				</p>
			{/if}
		</div>

		<aside class="rounded-xl border border-slate-800 bg-slate-900/50 p-4">
			<h2 class="mb-3 flex items-center gap-2 text-lg font-semibold">
				Anomaly Feed
				<span class="rounded-full bg-slate-800 px-2 py-0.5 text-xs text-slate-400">
					{anomalyList.length}
				</span>
			</h2>

			{#if anomalyList.length === 0}
				<p class="text-sm text-slate-500">
					No volume spikes detected yet. When a symbol's volume exceeds 3x its recent average, it
					appears here and is automatically analyzed for news sentiment.
				</p>
			{:else}
				<ul class="space-y-3">
					{#each anomalyList as a (a.event_id)}
						<li class="rounded-lg border border-slate-800 bg-slate-950 p-3">
							<div class="flex items-center justify-between">
								<span class="font-semibold text-sky-400">{a.symbol}</span>
								<span class="text-xs text-slate-500">{fmtTime(a.time)}</span>
							</div>
							<p class="mt-1 text-xs text-slate-400">
								Volume {a.trigger_volume.toLocaleString()} vs avg {Math.round(
									a.avg_volume
								).toLocaleString()}
							</p>
							<div class="mt-2 border-t border-slate-800 pt-2">
								<div class="flex items-center justify-between text-sm">
									<span class="text-slate-400">Sentiment</span>
									<span class="font-semibold {sentimentColor(a.sentiment_score)}">
										{sentimentLabel(a.sentiment_score)}
										{#if a.sentiment_score !== undefined}
											({a.sentiment_score.toFixed(2)})
										{/if}
									</span>
								</div>
								{#if a.top_headline}
									<p class="mt-1 text-xs text-slate-400">
										“{a.top_headline}”
									</p>
									<p class="mt-1 text-xs text-slate-600">{a.article_count} articles analyzed</p>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</aside>
	</main>
</div>
