# AutoStack Demo Environment
# Usage:
#   make demo-up      — Start the demo environment
#   make demo-down    — Stop and remove containers
#   make demo-reset   — Wipe all demo data and restart fresh
#   make demo-logs    — Tail all container logs
#   make demo-status  — Show container status

COMPOSE = docker compose
ENV_FILE = .env.demo

.PHONY: demo-up demo-down demo-reset demo-logs demo-status demo-verify

demo-up:
	@echo "Starting AutoStack demo environment..."
	@$(COMPOSE) up -d
	@echo "Seeding demo data..."
	@bash seed.sh
	@echo "Demo environment ready."

demo-down:
	@echo "Stopping AutoStack demo environment..."
	@$(COMPOSE) down

demo-reset:
	@echo "Resetting AutoStack demo environment (all data will be lost)..."
	@$(COMPOSE) down -v
	@$(COMPOSE) up -d
	@echo "Waiting for services..."
	@sleep 5
	@bash seed.sh
	@echo "Demo reset complete."

demo-logs:
	@$(COMPOSE) logs -f

demo-status:
	@$(COMPOSE) ps

demo-verify:
	@echo "Verifying demo environment health..."
	@curl -sf http://localhost:8090/api/health && echo "PocketBase: OK" || echo "PocketBase: FAIL"
	@curl -sf http://localhost:9090/-/healthy && echo "Prometheus: OK" || echo "Prometheus: FAIL"
	@curl -sf http://localhost:3000/api/health && echo "Grafana: OK" || echo "Grafana: FAIL"
	@echo "Verify complete."
