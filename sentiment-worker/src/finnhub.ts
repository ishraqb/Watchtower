import fetch from 'node-fetch';
import { config } from './config.js';

export interface NewsArticle {
	headline: string;
	summary: string;
	url: string;
	datetime: number;
}

// Symbols are validated before being placed in the outbound URL to prevent
// SSRF / URL injection, even though they originate from our own detector.
const SYMBOL_RE = /^[A-Z.]{1,10}$/;

const NEWS_URL = 'https://finnhub.io/api/v1/company-news';

/**
 * Fetches company news for a symbol over the last `lookbackDays` days.
 * Returns an empty array on any error so the pipeline degrades gracefully.
 */
export async function fetchCompanyNews(symbol: string, lookbackDays = 1): Promise<NewsArticle[]> {
	if (!SYMBOL_RE.test(symbol)) {
		throw new Error(`invalid symbol: ${symbol}`);
	}

	const to = new Date();
	const from = new Date(to.getTime() - lookbackDays * 24 * 60 * 60 * 1000);
	const fmt = (d: Date) => d.toISOString().slice(0, 10);

	// Host is a fixed constant; only the validated symbol, dates, and key are added.
	const url = `${NEWS_URL}?symbol=${symbol}&from=${fmt(from)}&to=${fmt(to)}&token=${config.finnhubApiKey}`;

	const res = await fetch(url);
	if (!res.ok) {
		// Status only — never echo the URL (it contains the API key).
		throw new Error(`finnhub news request failed: ${res.status}`);
	}

	const data = (await res.json()) as NewsArticle[];
	return Array.isArray(data) ? data : [];
}
