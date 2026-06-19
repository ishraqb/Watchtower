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
	kafkaBrokers: (process.env.KAFKA_BROKERS ?? 'localhost:9092').split(','),
	databaseUrl: required('DATABASE_URL'),
	finnhubApiKey: required('FINNHUB_API_KEY')
};

export const TOPIC_ANOMALIES = 'market-anomalies';
export const TOPIC_SENTIMENT = 'sentiment-results';
