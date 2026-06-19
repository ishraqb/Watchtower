.PHONY: up down logs ps restart clean backend worker frontend

# Start all infrastructure containers in the background
up:
	docker compose up -d

# Stop and remove all containers
down:
	docker compose down

# Tail logs from all containers
logs:
	docker compose logs -f

# Show running container status
ps:
	docker compose ps

# Restart all containers
restart: down up

# Stop containers and remove the TimescaleDB volume (DESTROYS DATA)
clean:
	docker compose down -v

# Run the Go backend locally
backend:
	cd backend && go run ./cmd/server

# Run the TypeScript sentiment worker locally
worker:
	cd sentiment-worker && npm run dev

# Run the SvelteKit frontend locally
frontend:
	cd frontend && npm run dev
