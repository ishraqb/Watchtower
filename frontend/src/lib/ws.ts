import { writable, type Writable } from 'svelte/store';

export interface Tick {
	type: string;
	symbol: string;
	price: number;
	volume: number;
	time: number; // epoch millis
}

export type ConnectionStatus = 'connecting' | 'open' | 'closed';

/** The most recent tick received, or null before the first message. */
export const latestTick: Writable<Tick | null> = writable(null);

/** Live connection status for the dashboard header. */
export const connectionStatus: Writable<ConnectionStatus> = writable('closed');

/** Latest known price keyed by symbol, for summary tiles. */
export const prices: Writable<Record<string, Tick>> = writable({});

let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

/**
 * Opens a WebSocket to the Go backend and streams ticks into the stores.
 * Auto-reconnects with a fixed backoff. Browser-only (guarded for SSR).
 */
export function connect(url = 'ws://localhost:8080/ws'): void {
	if (typeof window === 'undefined') return; // no WebSocket during SSR
	if (socket && socket.readyState <= WebSocket.OPEN) return;

	connectionStatus.set('connecting');
	socket = new WebSocket(url);

	socket.onopen = () => connectionStatus.set('open');

	socket.onmessage = (event) => {
		try {
			const tick = JSON.parse(event.data) as Tick;
			if (tick.type !== 'tick') return;
			latestTick.set(tick);
			prices.update((p) => ({ ...p, [tick.symbol]: tick }));
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
