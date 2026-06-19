// Loads configuration from the environment. For local dev it attempts to read
// the repo-root .env (Node >= 20.12); in Docker the values are injected directly.

try {
	process.loadEnvFile?.('../.env');
} catch {
	// .env is optional; rely on the ambient environment.
}

function required(name: string): string {
	const value = process.env[name];
	if (!value) {
		// Names only — never log secret values.
		console.warn(`config: warning — ${name} is not set`);
	}
	return value ?? '';
}

export const config = {
	// "kafka" (local/dev) or "redis" (Redis Streams, used on free hosting).
	broker: process.env.BROKER ?? 'kafka',
	kafkaBrokers: (process.env.KAFKA_BROKERS ?? 'localhost:9092').split(','),
	redisUrl: process.env.REDIS_URL ?? 'redis://localhost:6379',
	databaseUrl: required('DATABASE_URL'),
	finnhubApiKey: required('FINNHUB_API_KEY'),
	// Port for the little health/wake HTTP server (Render needs a listening port).
	port: Number(process.env.PORT ?? 3001)
};
