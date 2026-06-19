<script lang="ts">
	import { getJSON } from '$lib/api';

	interface CongressTrade {
		symbol: string;
		representative: string;
		transaction_date: string;
		transaction_type: 'BUY' | 'SELL';
		amount_range: string;
	}

	interface CongressResponse {
		symbol: string;
		trades: CongressTrade[];
	}

	const SYMBOLS = ['AAPL', 'TSLA', 'RIVN', 'NVDA', 'MSFT', 'AMZN', 'GOOGL', 'META'];

	let selected = $state('AAPL');
	let trades = $state<CongressTrade[]>([]);
	let loading = $state(false);
	let error = $state('');

	async function load(symbol: string) {
		loading = true;
		error = '';
		try {
			const data = await getJSON<CongressResponse>(`/api/congress/${symbol}`);
			trades = data.trades;
		} catch {
			error = 'Could not reach the backend. Is the Go server running on :8080?';
			trades = [];
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load(selected);
	});

	const buyCount = $derived(trades.filter((t) => t.transaction_type === 'BUY').length);
	const sellCount = $derived(trades.filter((t) => t.transaction_type === 'SELL').length);

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<svelte:head>
	<title>Watchtower — Congressional Trades</title>
</svelte:head>

<div class="min-h-screen bg-slate-950 text-slate-100">
	<header class="border-b border-slate-800 px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-bold tracking-tight">Congressional Trades</h1>
				<p class="text-sm text-slate-400">STOCK Act disclosures from the House and Senate</p>
			</div>
			<nav class="flex gap-4 text-sm">
				<a class="text-slate-400 hover:text-slate-200" href="/">Live</a>
				<a class="text-sky-400 hover:text-sky-300" href="/congress">Congress</a>
				<a class="text-slate-400 hover:text-slate-200" href="/ipo">IPOs</a>
			</nav>
		</div>
	</header>

	<main class="p-6">
		<div class="mb-6 flex flex-wrap gap-2">
			{#each SYMBOLS as sym (sym)}
				<button
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors {selected === sym
						? 'bg-sky-500 text-white'
						: 'bg-slate-800 text-slate-300 hover:bg-slate-700'}"
					onclick={() => (selected = sym)}
				>
					{sym}
				</button>
			{/each}
		</div>

		<div class="mb-4 flex gap-4">
			<div class="rounded-lg border border-slate-800 bg-slate-900/50 px-4 py-2">
				<span class="text-xs text-slate-400">Buys</span>
				<p class="text-xl font-bold text-emerald-400">{buyCount}</p>
			</div>
			<div class="rounded-lg border border-slate-800 bg-slate-900/50 px-4 py-2">
				<span class="text-xs text-slate-400">Sells</span>
				<p class="text-xl font-bold text-rose-400">{sellCount}</p>
			</div>
		</div>

		{#if loading}
			<p class="text-slate-400">Loading {selected} trades…</p>
		{:else if error}
			<p class="rounded-lg border border-rose-900 bg-rose-950/50 px-4 py-3 text-rose-300">{error}</p>
		{:else if trades.length === 0}
			<p class="text-slate-400">No disclosed trades found for {selected}.</p>
		{:else}
			<div class="overflow-hidden rounded-xl border border-slate-800">
				<table class="w-full text-left text-sm">
					<thead class="bg-slate-900 text-slate-400">
						<tr>
							<th class="px-4 py-3 font-medium">Representative</th>
							<th class="px-4 py-3 font-medium">Date</th>
							<th class="px-4 py-3 font-medium">Type</th>
							<th class="px-4 py-3 font-medium">Amount</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-slate-800">
						{#each trades as t (t.representative + t.transaction_date + t.transaction_type)}
							<tr class="bg-slate-950 hover:bg-slate-900/50">
								<td class="px-4 py-3">{t.representative}</td>
								<td class="px-4 py-3 text-slate-400">{fmtDate(t.transaction_date)}</td>
								<td class="px-4 py-3">
									<span
										class="rounded-full px-2 py-0.5 text-xs font-semibold {t.transaction_type ===
										'BUY'
											? 'bg-emerald-500/20 text-emerald-300'
											: 'bg-rose-500/20 text-rose-300'}"
									>
										{t.transaction_type}
									</span>
								</td>
								<td class="px-4 py-3 text-slate-300">{t.amount_range || '—'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</main>
</div>
