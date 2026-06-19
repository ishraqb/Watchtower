// Base URL of the Go backend REST API. Set VITE_API_BASE at build time for
// deployed environments; falls back to the local backend during development.
export const API_BASE =
	(import.meta.env.VITE_API_BASE as string | undefined) ?? 'http://localhost:8080';

export async function getJSON<T>(path: string): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`);
	if (!res.ok) {
		throw new Error(`request failed: ${res.status}`);
	}
	return (await res.json()) as T;
}

// Last-known price snapshot returned by GET /api/quote/:symbol.
export interface Quote {
	symbol: string;
	current: number;
	change: number;
	percent_change: number;
	high: number;
	low: number;
	open: number;
	previous_close: number;
}

export function getQuote(symbol: string): Promise<Quote> {
	return getJSON<Quote>(`/api/quote/${encodeURIComponent(symbol)}`);
}

// A single (unix-second, price) sample from GET /api/history.
export interface HistoryPoint {
	time: number;
	value: number;
}

export interface History {
	symbol: string;
	range: string;
	previous_close: number;
	points: HistoryPoint[];
}

export function getHistory(symbol: string, range: string): Promise<History> {
	return getJSON<History>(
		`/api/history/${encodeURIComponent(symbol)}?range=${encodeURIComponent(range)}`
	);
}

// Subscribes a symbol to the backend's live Finnhub WebSocket stream so it
// receives real-time ticks. Idempotent on the server.
export async function watchSymbol(symbol: string): Promise<void> {
	await fetch(`${API_BASE}/api/watch/${encodeURIComponent(symbol)}`, { method: 'POST' });
}
