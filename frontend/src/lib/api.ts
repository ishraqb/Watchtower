// Base URL of the Go backend REST API.
export const API_BASE = 'http://localhost:8080';

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
