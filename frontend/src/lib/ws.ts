import { writable, type Writable } from 'svelte/store';

export interface Tick {
	type: string;
	symbol: string;
	price: number;
	volume: number;
	time: number; // epoch millis
}

export type ConnectionStatus = 'connecting' | 'open' | 'closed';

/** A detected volume spike, with sentiment merged in once it arrives. */
export interface Anomaly {
	event_id: number;
	symbol: string;
	trigger_volume: number;
	avg_volume: number;
	time: number;
	sentiment_score?: number;
	article_count?: number;
	top_headline?: string;
}

/** The most recent tick received, or null before the first message. */
export const latestTick: Writable<Tick | null> = writable(null);

/** Live connection status for the dashboard header. */
export const connectionStatus: Writable<ConnectionStatus> = writable('closed');

/** Latest known price keyed by symbol, for summary tiles. */
export const prices: Writable<Record<string, Tick>> = writable({});

/** Detected anomalies, newest first, with sentiment merged in by event_id. */
export const anomalies: Writable<Anomaly[]> = writable([]);

const MAX_ANOMALIES = 25;

let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

// Backend WebSocket URL. Set VITE_WS_URL at build time for deployed
// environments; falls back to the local backend during development.
const DEFAULT_WS_URL =
	(import.meta.env.VITE_WS_URL as string | undefined) ?? 'ws://localhost:8080/ws';

/**
 * Opens a WebSocket to the Go backend and streams ticks into the stores.
 * Auto-reconnects with a fixed backoff. Browser-only (guarded for SSR).
 */
export function connect(url = DEFAULT_WS_URL): void {
	if (typeof window === 'undefined') return; // no WebSocket during SSR
	if (socket && socket.readyState <= WebSocket.OPEN) return;

	connectionStatus.set('connecting');
	socket = new WebSocket(url);

	socket.onopen = () => connectionStatus.set('open');

	socket.onmessage = (event) => {
		try {
			const msg = JSON.parse(event.data) as { type: string } & Record<string, unknown>;
			switch (msg.type) {
				case 'tick': {
					const tick = msg as unknown as Tick;
					latestTick.set(tick);
					prices.update((p) => ({ ...p, [tick.symbol]: tick }));
					break;
				}
				case 'anomaly': {
					const a = msg as unknown as Anomaly;
					anomalies.update((list) => [a, ...list].slice(0, MAX_ANOMALIES));
					break;
				}
				case 'sentiment': {
					const s = msg as unknown as Anomaly;
					anomalies.update((list) =>
						list.map((a) =>
							a.event_id === s.event_id
								? {
										...a,
										sentiment_score: s.sentiment_score,
										article_count: s.article_count,
										top_headline: s.top_headline
									}
								: a
						)
					);
					break;
				}
			}
		} catch {
			// Ignore malformed frames rather than crashing the stream.
		}
	};

	socket.onclose = () => {
		connectionStatus.set('closed');
		scheduleReconnect(url);
	};

	socket.onerror = () => {
		socket?.close();
	};
}

function scheduleReconnect(url: string): void {
	if (reconnectTimer) return;
	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		connect(url);
	}, 3000);
}

/** Closes the socket and cancels any pending reconnect. */
export function disconnect(): void {
	if (reconnectTimer) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}
	socket?.close();
	socket = null;
}
