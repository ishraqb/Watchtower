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
	import { getQuote, getHistory, type Quote } from '$lib/api';

	const DEFAULT_SYMBOLS = ['AAPL', 'TSLA', 'RIVN'];
	const STORAGE_KEY = 'watchtower:symbols';
	const MAX_SYMBOLS = 12;
	// How often to auto-refresh quotes/charts so non-streaming symbols stay live.
	const POLL_INTERVAL_MS = 20_000;

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

	let pollTimer: ReturnType<typeof setInterval> | null = null;

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
			// Guard against a stale poll landing after the user switched ranges.
			if (charts[sym] && selectedRange[sym] === range) {
				setSeriesData(charts[sym], pts);
			}
			if (pts.length > 0) {
				const last = pts[pts.length - 1].value;
				// 1D is measured against the prior close (Robinhood-style); other
				// ranges are measured against the first point in the window.
				const base =
					range === '1d' && hist.previous_close > 0 ? hist.previous_close : pts[0].value;
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
		if (change === undefined || change === 0) return 'text-slate-400';
		return change > 0 ? 'text-emerald-400' : 'text-rose-400';
	}

	// Periodic refresh so every symbol (including non-streaming, user-added ones)
	// updates automatically. Symbols actively streaming live ticks on an intraday
	// view are left to the WebSocket to avoid fighting the real-time series.
	function refreshAll() {
		for (const sym of symbols) {
			getQuote(sym)
				.then((q) => (quotes[sym] = q))
				.catch(() => {});
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
		pollTimer = setInterval(refreshAll, POLL_INTERVAL_MS);
	});

	onDestroy(() => {
		unsubStatus();
		unsubPrices();
		unsubTick();
		unsubAnomalies();
		if (pollTimer) clearInterval(pollTimer);
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
			<form class="mb-6 flex flex-wrap items-center gap-2" onsubmit={addSymbol}>
				<input
					class="w-40 rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm uppercase placeholder:normal-case placeholder:text-slate-500 focus:border-sky-500 focus:outline-none"
					placeholder="Add ticker (e.g. NVDA)"
					maxlength="10"
					bind:value={newSymbol}
				/>
				<button
					type="submit"
					class="rounded-lg bg-sky-500 px-4 py-2 text-sm font-medium text-white hover:bg-sky-400 disabled:opacity-50"
					disabled={symbols.length >= MAX_SYMBOLS}
				>
					Add
				</button>
				{#if addError}
					<span class="text-sm text-rose-400">{addError}</span>
				{:else}
					<span class="text-xs text-slate-500">
						Tracking {symbols.length}/{MAX_SYMBOLS} · your list is saved in this browser
					</span>
				{/if}
			</form>

			<div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
				{#each symbols as sym (sym)}
					<section class="rounded-xl border border-slate-800 bg-slate-900/50 p-4">
						<div class="mb-2 flex items-baseline justify-between">
							<div class="flex items-center gap-2">
								<h2 class="text-lg font-semibold">{sym}</h2>
								<button
									class="text-slate-600 hover:text-rose-400"
									title="Remove {sym}"
									aria-label="Remove {sym}"
									onclick={() => removeSymbol(sym)}
								>
									✕
								</button>
							</div>
							<div class="text-right">
								<span class="font-mono text-xl text-sky-400">
									{displayPrice(sym) !== undefined
										? `$${displayPrice(sym)!.toFixed(2)}`
										: '—'}
								</span>
								{#if rangeChange[sym]}
									<span class="ml-1 font-mono text-xs {changeColor(rangeChange[sym].pct)}">
										{rangeChange[sym].pct >= 0 ? '+' : ''}{rangeChange[sym].pct.toFixed(2)}%
										<span class="text-slate-500">{rangeLabel(sym)}</span>
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

						<div use:chartLifecycle={sym} class="h-64 w-full"></div>

						<div class="mt-2 flex flex-wrap justify-center gap-1">
							{#each RANGES as r (r.key)}
								<button
									class="rounded px-2 py-1 text-xs font-medium transition-colors
										{selectedRange[sym] === r.key
										? 'bg-sky-500 text-white'
										: 'bg-slate-800 text-slate-400 hover:bg-slate-700 hover:text-slate-200'}"
									onclick={() => loadRange(sym, r.key)}
								>
									{r.label}
								</button>
							{/each}
						</div>

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
