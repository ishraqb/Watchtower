<script lang="ts">
	import { getJSON } from '$lib/api';

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
		if (impact < 0) return 'text-emerald-300';
		if (impact > 0) return 'text-rose-300';
		return 'text-slate-300';
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
		if (score <= 33) return 'bg-emerald-500/20 text-emerald-300';
		if (score <= 66) return 'bg-amber-500/20 text-amber-300';
		return 'bg-rose-500/20 text-rose-300';
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

<div class="min-h-screen bg-slate-950 text-slate-100">
	<header class="border-b border-slate-800 px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-bold tracking-tight">IPO Risk Rater</h1>
				<p class="text-sm text-slate-400">
					Upcoming listings scored by offering size and pricing · click a row to see why
				</p>
			</div>
			<nav class="flex gap-4 text-sm">
				<a class="text-slate-400 hover:text-slate-200" href="/">Live</a>
				<a class="text-slate-400 hover:text-slate-200" href="/congress">Congress</a>
				<a class="text-sky-400 hover:text-sky-300" href="/ipo">IPOs</a>
			</nav>
		</div>
	</header>

	<main class="p-6">
		{#if loading}
			<p class="text-slate-400">Loading upcoming IPOs…</p>
		{:else if error}
			<p class="rounded-lg border border-rose-900 bg-rose-950/50 px-4 py-3 text-rose-300">{error}</p>
		{:else if sorted.length === 0}
			<p class="text-slate-400">No upcoming IPOs found. The daily poll may not have run yet.</p>
		{:else}
			<div class="overflow-hidden rounded-xl border border-slate-800">
				<table class="w-full text-left text-sm">
					<thead class="bg-slate-900 text-slate-400">
						<tr>
							<th class="px-4 py-3 font-medium">Symbol</th>
							<th class="px-4 py-3 font-medium">Company</th>
							<th class="px-4 py-3 font-medium">Exchange</th>
							<th class="px-4 py-3 font-medium">
								<button class="hover:text-slate-200" onclick={() => setSort('expected_date')}>
									Expected {sortKey === 'expected_date' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
							<th class="px-4 py-3 font-medium">Price</th>
							<th class="px-4 py-3 font-medium">
								<button class="hover:text-slate-200" onclick={() => setSort('shares_value')}>
									Raise {sortKey === 'shares_value' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
							<th class="px-4 py-3 font-medium">
								<button class="hover:text-slate-200" onclick={() => setSort('risk_score')}>
									Risk {sortKey === 'risk_score' ? (sortAsc ? '↑' : '↓') : ''}
								</button>
							</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-slate-800">
						{#each sorted as ipo (ipo.symbol + (ipo.expected_date ?? ''))}
							<tr
								class="cursor-pointer bg-slate-950 hover:bg-slate-900/50"
								onclick={() => openDetail(ipo)}
							>
								<td class="px-4 py-3 font-mono font-semibold text-sky-400">{ipo.symbol}</td>
								<td class="px-4 py-3">{ipo.company_name}</td>
								<td class="px-4 py-3 text-slate-400">{ipo.exchange || '—'}</td>
								<td class="px-4 py-3 text-slate-400">{fmtDate(ipo.expected_date)}</td>
								<td class="px-4 py-3 text-slate-300">{ipo.price_range || '—'}</td>
								<td class="px-4 py-3 text-slate-300">{fmtValue(ipo.shares_value)}</td>
								<td class="px-4 py-3">
									<span
										class="rounded-full px-2 py-0.5 text-xs font-semibold {riskClasses(
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
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) closeDetail();
		}}
	>
		<div
			class="max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl"
			role="dialog"
			aria-modal="true"
			aria-label="IPO rating details"
			tabindex="-1"
		>
			<div class="flex items-start justify-between">
				<div>
					<h2 class="font-mono text-xl font-semibold text-sky-400">{selected.symbol}</h2>
					<p class="text-sm text-slate-300">{selected.company_name}</p>
				</div>
				<button
					class="rounded-md px-2 py-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
					onclick={closeDetail}
					aria-label="Close"
				>
					✕
				</button>
			</div>

			<div class="mt-4 grid grid-cols-3 gap-3 text-sm">
				<div class="rounded-lg bg-slate-950 p-3">
					<div class="text-xs text-slate-500">Exchange</div>
					<div class="text-slate-200">{selected.exchange || '—'}</div>
				</div>
				<div class="rounded-lg bg-slate-950 p-3">
					<div class="text-xs text-slate-500">Price</div>
					<div class="text-slate-200">{selected.price_range || '—'}</div>
				</div>
				<div class="rounded-lg bg-slate-950 p-3">
					<div class="text-xs text-slate-500">Raise</div>
					<div class="text-slate-200">{fmtValue(selected.shares_value)}</div>
				</div>
			</div>

			<div class="mt-5 flex items-center justify-between">
				<span class="text-sm text-slate-400">Risk rating</span>
				<span
					class="rounded-full px-3 py-1 text-sm font-semibold {riskClasses(selected.risk_score)}"
				>
					{riskLabel(selected.risk_score)} · {selected.risk_score} / 100
				</span>
			</div>

			<h3 class="mt-5 mb-2 text-sm font-semibold text-slate-200">How this score was calculated</h3>
			<ul class="space-y-2">
				{#each selected.factors as f (f.label + f.detail)}
					<li class="rounded-lg border border-slate-800 bg-slate-950 p-3">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-slate-200">{f.label}</span>
							<span class="font-mono text-sm font-semibold {impactColor(f.impact)}">
								{f.impact > 0 ? '+' : ''}{f.impact}
							</span>
						</div>
						<p class="mt-1 text-xs text-slate-400">{f.detail}</p>
					</li>
				{/each}
			</ul>
			<p class="mt-3 text-xs text-slate-500">
				Higher scores mean higher risk. Green factors reduce risk; red factors increase it.
			</p>
		</div>
	</div>
{/if}
