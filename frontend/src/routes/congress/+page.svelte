<script lang="ts">
	import { getJSON } from '$lib/api';
	import Nav from '$lib/Nav.svelte';

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

<div class="min-h-screen">
	<Nav active="congress" />

	<main class="mx-auto max-w-5xl px-6 py-8">
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight text-white">Congressional Trades</h1>
			<p class="mt-1 text-sm text-[#9a9a9a]">STOCK Act disclosures from the House and Senate</p>
		</div>

		<div class="mb-6 flex flex-wrap gap-2">
			{#each SYMBOLS as sym (sym)}
				<button
					class="rounded-full px-4 py-1.5 text-sm font-semibold transition-colors {selected === sym
						? 'bg-white text-black'
						: 'bg-[#1a1a1a] text-[#9a9a9a] hover:bg-[#242424] hover:text-white'}"
					onclick={() => (selected = sym)}
				>
					{sym}
				</button>
			{/each}
		</div>

		<div class="mb-6 flex gap-3">
			<div class="flex-1 rounded-2xl bg-[#141414] px-5 py-4">
				<span class="text-xs uppercase tracking-wide text-[#6a6a6a]">Buys</span>
				<p class="tnum text-2xl font-bold text-[#00c805]">{buyCount}</p>
			</div>
			<div class="flex-1 rounded-2xl bg-[#141414] px-5 py-4">
				<span class="text-xs uppercase tracking-wide text-[#6a6a6a]">Sells</span>
				<p class="tnum text-2xl font-bold text-[#ff5000]">{sellCount}</p>
			</div>
		</div>

		{#if loading}
			<p class="text-[#9a9a9a]">Loading {selected} trades…</p>
		{:else if error}
			<p class="rounded-xl bg-[#1a0d0a] px-4 py-3 text-[#ff5000]">{error}</p>
		{:else if trades.length === 0}
			<p class="text-[#9a9a9a]">No disclosed trades found for {selected}.</p>
		{:else}
			<div class="overflow-hidden rounded-2xl bg-[#141414]">
				<table class="w-full text-left text-sm">
					<thead class="text-[#6a6a6a]">
						<tr class="border-b border-[#222]">
							<th class="px-5 py-3 font-medium">Representative</th>
							<th class="px-5 py-3 font-medium">Date</th>
							<th class="px-5 py-3 font-medium">Type</th>
							<th class="px-5 py-3 font-medium">Amount</th>
						</tr>
					</thead>
					<tbody>
						{#each trades as t (t.representative + t.transaction_date + t.transaction_type)}
							<tr class="border-b border-[#1c1c1c] transition-colors last:border-0 hover:bg-[#181818]">
								<td class="px-5 py-3 text-white">{t.representative}</td>
								<td class="tnum px-5 py-3 text-[#9a9a9a]">{fmtDate(t.transaction_date)}</td>
								<td class="px-5 py-3 font-semibold {t.transaction_type === 'BUY'
									? 'text-[#00c805]'
									: 'text-[#ff5000]'}">
									{t.transaction_type}
								</td>
								<td class="tnum px-5 py-3 text-[#cfcfcf]">{t.amount_range || '—'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</main>
</div>
