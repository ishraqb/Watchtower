// Base URL of the Go backend REST API.
export const API_BASE = 'http://localhost:8080';

export async function getJSON<T>(path: string): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`);
	if (!res.ok) {
		throw new Error(`request failed: ${res.status}`);
	}
	return (await res.json()) as T;
}
