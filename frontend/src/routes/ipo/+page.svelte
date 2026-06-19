<script lang="ts">
	import { getJSON } from '$lib/api';
	import Nav from '$lib/Nav.svelte';

	interface RiskFactor {
		label: string;
		impact: number;
		detail: string;
	}

	interface IPOEvaluation {
		symbol: string;
		company_name: string;
		expected_date: string | null;
		risk_score: number;
		exchange: string;
		price_range: string;
		shares_value: number;
		factors: RiskFactor[];
	}

	interface IPOResponse {
		ipos: IPOEvaluation[];
	}

	type SortKey = 'expected_date' | 'risk_score' | 'shares_value';

	let ipos = $state<IPOEvaluation[]>([]);
	let loading = $state(true);
	let error = $state('');
	let sortKey = $state<SortKey>('expected_date');
	let sortAsc = $state(true);
	let selected = $state<IPOEvaluation | null>(null);

	function openDetail(ipo: IPOEvaluation) {
		selected = ipo;
	}

	function closeDetail() {
		selected = null;
	}

	function impactColor(impact: number): string {
		if (impact < 0) return 'text-[#00c805]';
		if (impact > 0) return 'text-[#ff5000]';
		return 'text-[#cfcfcf]';
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const data = await getJSON<IPOResponse>('/api/ipo');
			ipos = data.ipos;
		} catch {
			error = 'Could not reach the backend. Is the Go server running on :8080?';
			ipos = [];
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	const sorted = $derived(
		[...ipos].sort((a, b) => {
			let cmp = 0;
			if (sortKey === 'expected_date') {
				cmp = (a.expected_date ?? '').localeCompare(b.expected_date ?? '');
			} else {
				cmp = a[sortKey] - b[sortKey];
			}
			return sortAsc ? cmp : -cmp;
		})
	);

	function setSort(key: SortKey) {
		if (sortKey === key) {
			sortAsc = !sortAsc;
		} else {
			sortKey = key;
			sortAsc = true;
		}
	}

	function riskClasses(score: number): string {
		if (score <= 33) return 'bg-[#00c805]/15 text-[#00c805]';
		if (score <= 66) return 'bg-[#e8b923]/15 text-[#e8b923]';
		return 'bg-[#ff5000]/15 text-[#ff5000]';
	}

	function riskLabel(score: number): string {
		if (score <= 33) return 'Low';
		if (score <= 66) return 'Medium';
		return 'High';
	}

	function fmtDate(iso: string | null): string {
		if (!iso) return 'TBD';
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function fmtValue(v: number): string {
		if (!v) return '—';
		if (v >= 1e9) return `$${(v / 1e9).toFixed(1)}B`;
		if (v >= 1e6) return `$${(v / 1e6).toFixed(0)}M`;
		return `$${v.toLocaleString()}`;
	}
</script>

<svelte:head>
	<title>Watchtower — IPO Risk Rater</title>
</svelte:head>

<div class="min-h-screen">
	<Nav active="ipo" />

	<main class="mx-auto max-w-5xl px-6 py-8">
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight text-white">IPO Risk Rater</h1>
			<p class="mt-1 text-sm text-[#9a9a9a]">
				Upcoming listings scored by offering size and pricing · tap a row to see why
			</p>
		</div>

		{#if loading}
			<p class="text-[#9a9a9a]">Loading upcoming IPOs…</p>
		{:else if error}
			<p class="rounded-xl bg-[#1a0d0a] px-4 py-3 text-[#ff5000]">{error}</p>
		{:else if sorted.length === 0}
			<p class="text-[#9a9a9a]">No upcoming IPOs found. The daily poll may not have run yet.</p>
		{:else}
			<div class="overflow-hidden rounded-2xl bg-[#141414]">
				<table class="w-full text-left text-sm">
					<thead class="text-[#6a6a6a]">
						<tr class="border-b border-[#222]">
							<th class="px-5 py-3 font-medium">Symbol</th>
							<th class="px-5 py-3 font-medium">Company</th>
							<th class="px-5 py-3 font-medium">Exchange</th>
							<th class="px-5 py-3 font-medium">
								<button class="hover:text-white" onclick={() => setSort('expected_date')}>
									Expected {sortKey === 'expected_date' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
							<th class="px-5 py-3 font-medium">Price</th>
							<th class="px-5 py-3 font-medium">
								<button class="hover:text-white" onclick={() => setSort('shares_value')}>
									Raise {sortKey === 'shares_value' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
							<th class="px-5 py-3 font-medium">
								<button class="hover:text-white" onclick={() => setSort('risk_score')}>
									Risk {sortKey === 'risk_score' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
						</tr>
					</thead>
					<tbody>
						{#each sorted as ipo (ipo.symbol + (ipo.expected_date ?? ''))}
							<tr
								class="cursor-pointer border-b border-[#1c1c1c] transition-colors last:border-0 hover:bg-[#181818]"
								onclick={() => openDetail(ipo)}
							>
								<td class="px-5 py-3 font-bold text-white">{ipo.symbol}</td>
								<td class="px-5 py-3 text-[#cfcfcf]">{ipo.company_name}</td>
								<td class="px-5 py-3 text-[#9a9a9a]">{ipo.exchange || '—'}</td>
								<td class="tnum px-5 py-3 text-[#9a9a9a]">{fmtDate(ipo.expected_date)}</td>
								<td class="tnum px-5 py-3 text-[#cfcfcf]">{ipo.price_range || '—'}</td>
								<td class="tnum px-5 py-3 text-[#cfcfcf]">{fmtValue(ipo.shares_value)}</td>
								<td class="px-5 py-3">
									<span
										class="tnum inline-block whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-semibold {riskClasses(
											ipo.risk_score
										)}"
									>
										{riskLabel(ipo.risk_score)} · {ipo.risk_score}
									</span>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</main>
</div>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape') closeDetail();
	}}
/>

{#if selected}
	<!-- Backdrop: clicking the backdrop itself (not the panel) closes the modal -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) closeDetail();
		}}
	>
		<div
			class="max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-2xl border border-[#222] bg-[#141414] p-6 shadow-2xl"
			role="dialog"
			aria-modal="true"
			aria-label="IPO rating details"
			tabindex="-1"
		>
			<div class="flex items-start justify-between">
				<div>
					<h2 class="text-2xl font-bold text-white">{selected.symbol}</h2>
					<p class="text-sm text-[#9a9a9a]">{selected.company_name}</p>
				</div>
				<button
					class="rounded-md px-2 py-1 text-[#9a9a9a] hover:bg-[#222] hover:text-white"
					onclick={closeDetail}
					aria-label="Close"
				>
					✕
				</button>
			</div>

			<div class="mt-5 grid grid-cols-3 gap-3 text-sm">
				<div class="rounded-xl bg-[#0d0d0d] p-3">
					<div class="text-xs text-[#6a6a6a]">Exchange</div>
					<div class="text-white">{selected.exchange || '—'}</div>
				</div>
				<div class="rounded-xl bg-[#0d0d0d] p-3">
					<div class="text-xs text-[#6a6a6a]">Price</div>
					<div class="tnum text-white">{selected.price_range || '—'}</div>
				</div>
				<div class="rounded-xl bg-[#0d0d0d] p-3">
					<div class="text-xs text-[#6a6a6a]">Raise</div>
					<div class="tnum text-white">{fmtValue(selected.shares_value)}</div>
				</div>
			</div>

			<div class="mt-5 flex items-center justify-between">
				<span class="text-sm text-[#9a9a9a]">Risk rating</span>
				<span
					class="tnum rounded-full px-3 py-1 text-sm font-semibold {riskClasses(selected.risk_score)}"
				>
					{riskLabel(selected.risk_score)} · {selected.risk_score} / 100
				</span>
			</div>

			<h3 class="mt-5 mb-2 text-sm font-semibold text-white">How this score was calculated</h3>
			<ul class="space-y-2">
				{#each selected.factors as f (f.label + f.detail)}
					<li class="rounded-xl bg-[#0d0d0d] p-3">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-white">{f.label}</span>
							<span class="tnum text-sm font-semibold {impactColor(f.impact)}">
								{f.impact > 0 ? '+' : ''}{f.impact}
							</span>
						</div>
						<p class="mt-1 text-xs text-[#9a9a9a]">{f.detail}</p>
					</li>
				{/each}
			</ul>
			<p class="mt-3 text-xs text-[#6a6a6a]">
				Higher scores mean higher risk. Green factors reduce risk; red factors increase it.
			</p>
		</div>
	</div>
{/if}
