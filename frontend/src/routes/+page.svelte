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
	import { createSymbolChart, pushPrice, setSeriesData, type SymbolChart } from '$lib/charts';
	import { getQuote, getHistory, watchSymbol, type Quote } from '$lib/api';
	import Nav from '$lib/Nav.svelte';

	const DEFAULT_SYMBOLS = ['AAPL', 'TSLA', 'RIVN'];
	const STORAGE_KEY = 'watchtower:symbols';
	const MAX_SYMBOLS = 25;
	// Charts poll Yahoo (unmetered); quotes poll Finnhub, which is rate-limited,
	// so they refresh more slowly to stay under the free-tier 60 calls/min even
	// at 25 symbols. Live prices come from the WebSocket regardless.
	const CHART_POLL_MS = 30_000;
	const QUOTE_POLL_MS = 60_000;

	// Robinhood-style ranges. `key` matches the backend allow-list.
	const RANGES = [
		{ key: '1h', label: '1H' },
		{ key: '1d', label: '1D' },
		{ key: '1w', label: '1W' },
		{ key: 'ytd', label: 'YTD' },
		{ key: '1y', label: '1Y' },
		{ key: '5y', label: '5Y' },
		{ key: 'max', label: 'MAX' }
	];

	// Plain (non-reactive) chart registry, managed via a Svelte action so charts
	// are created/destroyed exactly when a symbol card mounts/unmounts.
	const charts: Record<string, SymbolChart> = {};

	let symbols = $state<string[]>([...DEFAULT_SYMBOLS]);
	let newSymbol = $state('');
	let addError = $state('');

	let status = $state<'connecting' | 'open' | 'closed'>('closed');
	let latestPrices = $state<Record<string, Tick>>({});
	let anomalyList = $state<Anomaly[]>([]);
	let quotes = $state<Record<string, Quote>>({});
	let selectedRange = $state<Record<string, string>>({});
	let rangeLoading = $state<Record<string, boolean>>({});
	// Net change over the currently-selected range, keyed by symbol.
	let rangeChange = $state<Record<string, { change: number; pct: number }>>({});

	let chartTimer: ReturnType<typeof setInterval> | null = null;
	let quoteTimer: ReturnType<typeof setInterval> | null = null;

	function persistSymbols() {
		if (typeof window === 'undefined') return;
		try {
			window.localStorage.setItem(STORAGE_KEY, JSON.stringify(symbols));
		} catch {
			// Storage may be unavailable (private mode); non-fatal.
		}
	}

	// Whether live ticks should append to a symbol's chart (only on intraday views).
	function isIntraday(sym: string): boolean {
		return selectedRange[sym] === '1d' || selectedRange[sym] === '1h';
	}

	const rangeLabel = $derived(
		(sym: string) => RANGES.find((r) => r.key === selectedRange[sym])?.label ?? ''
	);

	// Fetches history for a range, redraws the chart, and computes the period %.
	async function fetchAndRender(sym: string, range: string, showLoading: boolean) {
		if (showLoading) rangeLoading[sym] = true;
		try {
			const hist = await getHistory(sym, range);
			const pts = hist.points;
			// 1D is measured against the prior close (Robinhood-style); other
			// ranges are measured against the first point in the window.
			const base =
				range === '1d' && hist.previous_close > 0 ? hist.previous_close : (pts[0]?.value ?? 0);
			// Guard against a stale poll landing after the user switched ranges.
			if (charts[sym] && selectedRange[sym] === range) {
				setSeriesData(charts[sym], pts, base);
			}
			if (pts.length > 0) {
				const last = pts[pts.length - 1].value;
				const change = last - base;
				rangeChange[sym] = { change, pct: base ? (change / base) * 100 : 0 };
			}
		} catch {
			// Leave the existing chart in place on failure.
		} finally {
			if (showLoading) rangeLoading[sym] = false;
		}
	}

	function loadRange(sym: string, range: string) {
		selectedRange[sym] = range;
		fetchAndRender(sym, range, true);
	}

	// Loads a symbol's quote + initial chart. Called by the chart action on mount.
	function initSymbol(sym: string) {
		if (!selectedRange[sym]) selectedRange[sym] = '1d';
		getQuote(sym)
			.then((q) => (quotes[sym] = q))
			.catch(() => {});
		fetchAndRender(sym, selectedRange[sym], true);
		// Ask the backend to stream this symbol live (idempotent server-side).
		watchSymbol(sym).catch(() => {});
	}

	// Svelte action: owns the chart lifecycle for a single symbol card.
	function chartLifecycle(node: HTMLDivElement, sym: string) {
		charts[sym] = createSymbolChart(node);
		initSymbol(sym);
		return {
			destroy() {
				charts[sym]?.chart.remove();
				delete charts[sym];
			}
		};
	}

	async function addSymbol(e: Event) {
		e.preventDefault();
		addError = '';
		const sym = newSymbol.trim().toUpperCase();
		if (!/^[A-Z.]{1,10}$/.test(sym)) {
			addError = 'Enter a valid ticker (letters only, e.g. NVDA).';
			return;
		}
		if (symbols.includes(sym)) {
			addError = `${sym} is already on your watchlist.`;
			return;
		}
		if (symbols.length >= MAX_SYMBOLS) {
			addError = `You can track up to ${MAX_SYMBOLS} symbols.`;
			return;
		}
		// Validate the ticker actually exists before adding it.
		try {
			const q = await getQuote(sym);
			if (!q || (q.current === 0 && q.previous_close === 0)) {
				addError = `Couldn't find a stock with ticker "${sym}".`;
				return;
			}
			quotes[sym] = q;
		} catch {
			addError = 'Could not verify that symbol. Please try again.';
			return;
		}
		selectedRange[sym] = '1d';
		symbols = [...symbols, sym];
		persistSymbols();
		newSymbol = '';
	}

	function removeSymbol(sym: string) {
		symbols = symbols.filter((s) => s !== sym);
		persistSymbols();
		delete quotes[sym];
		delete selectedRange[sym];
		delete rangeChange[sym];
		delete rangeLoading[sym];
	}

	// Whether any live ticks have been received this session.
	const hasLiveTicks = $derived(Object.keys(latestPrices).length > 0);

	// The price shown per symbol: live tick if available, otherwise last-known quote.
	function displayPrice(sym: string): number | undefined {
		if (latestPrices[sym]) return latestPrices[sym].price;
		if (quotes[sym]) return quotes[sym].current;
		return undefined;
	}

	function changeColor(change: number | undefined): string {
		if (change === undefined || change === 0) return 'text-[#9a9a9a]';
		return change > 0 ? 'text-[#00c805]' : 'text-[#ff5000]';
	}

	// Refreshes day-stat quotes for every symbol. Runs on the slower interval
	// because quotes hit the rate-limited Finnhub REST API.
	function refreshQuotes() {
		for (const sym of symbols) {
			getQuote(sym)
				.then((q) => (quotes[sym] = q))
				.catch(() => {});
		}
	}

	// Refreshes charts for symbols that aren't already streaming live intraday
	// ticks over the WebSocket (those are kept real-time by the socket instead).
	function refreshCharts() {
		for (const sym of symbols) {
			const streamingIntraday = status === 'open' && !!latestPrices[sym] && isIntraday(sym);
			if (!streamingIntraday) {
				fetchAndRender(sym, selectedRange[sym] ?? '1d', false);
			}
		}
	}

	const unsubStatus = connectionStatus.subscribe((s) => (status = s));
	const unsubPrices = prices.subscribe((p) => (latestPrices = p));
	const unsubAnomalies = anomalies.subscribe((a) => (anomalyList = a));

	const unsubTick = latestTick.subscribe((tick) => {
		if (!tick) return;
		// Only extend the chart on intraday ranges; longer ranges are historical.
		if (!isIntraday(tick.symbol)) return;
		const sc = charts[tick.symbol];
		if (sc) pushPrice(sc, tick.time, tick.price);
	});

	function sentimentColor(score: number | undefined): string {
		if (score === undefined) return 'text-[#9a9a9a]';
		if (score > 0.05) return 'text-[#00c805]';
		if (score < -0.05) return 'text-[#ff5000]';
		return 'text-[#e8b923]';
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
		// Restore the user's saved watchlist (charts are created by the action).
		try {
			const raw = window.localStorage.getItem(STORAGE_KEY);
			if (raw) {
				const saved = JSON.parse(raw);
				if (Array.isArray(saved) && saved.every((s) => typeof s === 'string') && saved.length) {
					symbols = saved.slice(0, MAX_SYMBOLS);
				}
			}
		} catch {
			// Fall back to defaults on any parse/storage error.
		}
		connect();
		chartTimer = setInterval(refreshCharts, CHART_POLL_MS);
		quoteTimer = setInterval(refreshQuotes, QUOTE_POLL_MS);
	});

	onDestroy(() => {
		unsubStatus();
		unsubPrices();
		unsubTick();
		unsubAnomalies();
		if (chartTimer) clearInterval(chartTimer);
		if (quoteTimer) clearInterval(quoteTimer);
		disconnect();
	});

	const statusColor = $derived(
		status === 'open'
			? 'bg-[#00c805]'
			: status === 'connecting'
				? 'bg-[#e8b923]'
				: 'bg-[#ff5000]'
	);

	const statusLabel = $derived(
		status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Offline'
	);
</script>

<svelte:head>
	<title>Watchtower — Live Market Dashboard</title>
</svelte:head>

<div class="min-h-screen">
	<Nav active="live" />

	<main class="mx-auto grid max-w-6xl grid-cols-1 gap-8 px-6 py-8 xl:grid-cols-[1fr_20rem]">
		<div>
			<div class="mb-8 flex flex-wrap items-end justify-between gap-4">
				<div>
					<h1 class="text-3xl font-bold tracking-tight text-white">Watchlist</h1>
					<p class="mt-1 flex items-center gap-2 text-sm text-[#9a9a9a]">
						<span class="h-2 w-2 rounded-full {statusColor}"></span>
						{statusLabel} · real-time market intelligence
					</p>
				</div>
				<form class="flex items-center gap-2" onsubmit={addSymbol}>
					<input
						class="w-44 rounded-full border border-[#2a2a2a] bg-[#141414] px-4 py-2 text-sm uppercase text-white placeholder:normal-case placeholder:text-[#6a6a6a] focus:border-[#00c805] focus:outline-none"
						placeholder="Add symbol"
						maxlength="10"
						bind:value={newSymbol}
					/>
					<button
						type="submit"
						class="rounded-full bg-[#00c805] px-5 py-2 text-sm font-semibold text-black transition-opacity hover:opacity-90 disabled:opacity-40"
						disabled={symbols.length >= MAX_SYMBOLS}
					>
						Add
					</button>
				</form>
			</div>

			{#if addError}
				<p class="mb-4 text-sm text-[#ff5000]">{addError}</p>
			{/if}

			<div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
				{#each symbols as sym (sym)}
					<section
						class="group rounded-2xl bg-[#141414] p-5 transition-colors hover:bg-[#181818]"
					>
						<div class="mb-3 flex items-start justify-between">
							<div>
								<div class="flex items-center gap-2">
									<h2 class="text-xl font-bold text-white">{sym}</h2>
									<button
										class="text-[#3a3a3a] opacity-0 transition group-hover:opacity-100 hover:text-[#ff5000]"
										title="Remove {sym}"
										aria-label="Remove {sym}"
										onclick={() => removeSymbol(sym)}
									>
										✕
									</button>
								</div>
								{#if rangeChange[sym]}
									<div class="tnum mt-0.5 text-sm font-semibold {changeColor(rangeChange[sym].pct)}">
										{rangeChange[sym].pct >= 0 ? '▲' : '▼'}
										{Math.abs(rangeChange[sym].pct).toFixed(2)}%
										<span class="font-normal text-[#6a6a6a]">{rangeLabel(sym)}</span>
									</div>
								{/if}
							</div>
							<div class="text-right">
								<div class="tnum text-2xl font-bold text-white">
									{displayPrice(sym) !== undefined ? `$${displayPrice(sym)!.toFixed(2)}` : '—'}
								</div>
								{#if !latestPrices[sym] && quotes[sym]}
									<div class="text-[10px] uppercase tracking-wide text-[#6a6a6a]">at last close</div>
								{/if}
							</div>
						</div>

						<div use:chartLifecycle={sym} class="h-56 w-full"></div>

						<div class="mt-3 flex items-center justify-between border-t border-[#222] pt-3">
							{#each RANGES as r (r.key)}
								<button
									class="tnum rounded px-2 py-1 text-xs font-semibold transition-colors
										{selectedRange[sym] === r.key
										? 'text-[#00c805]'
										: 'text-[#6a6a6a] hover:text-white'}"
									onclick={() => loadRange(sym, r.key)}
								>
									{r.label}
								</button>
							{/each}
						</div>

						{#if quotes[sym]}
							<div class="tnum mt-3 grid grid-cols-4 gap-2 text-center text-[11px]">
								<div>
									<div class="text-[#6a6a6a]">Open</div>
									<div class="text-[#cfcfcf]">{quotes[sym].open.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-[#6a6a6a]">High</div>
									<div class="text-[#cfcfcf]">{quotes[sym].high.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-[#6a6a6a]">Low</div>
									<div class="text-[#cfcfcf]">{quotes[sym].low.toFixed(2)}</div>
								</div>
								<div>
									<div class="text-[#6a6a6a]">Prev</div>
									<div class="text-[#cfcfcf]">{quotes[sym].previous_close.toFixed(2)}</div>
								</div>
							</div>
						{/if}
					</section>
				{/each}
			</div>

			{#if !hasLiveTicks}
				<p class="mt-6 text-center text-xs text-[#6a6a6a]">
					Markets are quiet right now — showing each symbol's last-known price and day stats.
					Live charts resume automatically during US trading hours (9:30 AM–4:00 PM ET).
				</p>
			{/if}
		</div>

		<aside class="h-fit rounded-2xl bg-[#141414] p-5 xl:sticky xl:top-24">
			<h2 class="mb-4 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-[#9a9a9a]">
				Anomaly Feed
				<span class="rounded-full bg-[#222] px-2 py-0.5 text-xs text-[#9a9a9a]">
					{anomalyList.length}
				</span>
			</h2>

			{#if anomalyList.length === 0}
				<p class="text-sm leading-relaxed text-[#6a6a6a]">
					No volume spikes detected yet. When a symbol's volume exceeds 3× its recent average, it
					appears here and is automatically analyzed for news sentiment.
				</p>
			{:else}
				<ul class="space-y-3">
					{#each anomalyList as a (a.event_id)}
						<li class="rounded-xl bg-[#0d0d0d] p-3">
							<div class="flex items-center justify-between">
								<span class="font-bold text-white">{a.symbol}</span>
								<span class="text-xs text-[#6a6a6a]">{fmtTime(a.time)}</span>
							</div>
							<p class="tnum mt-1 text-xs text-[#9a9a9a]">
								Volume {a.trigger_volume.toLocaleString()} vs avg {Math.round(
									a.avg_volume
								).toLocaleString()}
							</p>
							<div class="mt-2 border-t border-[#222] pt-2">
								<div class="flex items-center justify-between text-sm">
									<span class="text-[#9a9a9a]">Sentiment</span>
									<span class="font-semibold {sentimentColor(a.sentiment_score)}">
										{sentimentLabel(a.sentiment_score)}
										{#if a.sentiment_score !== undefined}
											({a.sentiment_score.toFixed(2)})
										{/if}
									</span>
								</div>
								{#if a.top_headline}
									<p class="mt-1 text-xs text-[#9a9a9a]">
										“{a.top_headline}”
									</p>
									<p class="mt-1 text-xs text-[#5a5a5a]">{a.article_count} articles analyzed</p>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</aside>
	</main>
</div>
