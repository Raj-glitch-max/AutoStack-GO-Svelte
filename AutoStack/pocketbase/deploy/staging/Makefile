COMPOSE = docker compose -f deploy/staging/docker-compose.yaml
PROJECT = autostack-staging

.PHONY: staging-up staging-down staging-reset staging-logs staging-status staging-verify

staging-up:
	@echo "Building AutoStack staging images..."
	$(COMPOSE) build platform-api pocketbase
	@echo "Starting staging stack..."
	$(COMPOSE) up -d
	@echo "Waiting for platform-api healthcheck..."
	@timeout 60 sh -c 'until docker compose -f deploy/staging/docker-compose.yaml ps | grep platform-api | grep healthy; do sleep 2; done' || echo "WARN: platform-api not healthy after 60s"
	@echo "Staging stack is up."
	@echo "  platform-api: http://localhost:8090"
	@echo "  pocketbase:   http://localhost:8080"
	@echo "  prometheus:   http://localhost:9090"
	@echo "  grafana:      http://localhost:3000 (admin/autostack-staging)"

staging-down:
	@echo "Stopping staging stack..."
	$(COMPOSE) down
	@echo "Staging stack stopped."

staging-reset:
	@echo "Resetting staging stack (removes volumes)..."
	$(COMPOSE) down -v
	@echo "Staging reset complete."

staging-logs:
	$(COMPOSE) logs -f

staging-status:
	$(COMPOSE) ps

staging-verify:
	@echo "Verifying platform-api /healthz..."
	@curl -sf http://localhost:8090/healthz && echo "PASS: /healthz" || echo "FAIL: /healthz"
	@echo "Verifying platform-api /api/v1/readiness..."
	@curl -sf -H "X-Tenant-ID: staging-tenant" http://localhost:8090/api/v1/readiness && echo "PASS: /readiness" || echo "FAIL: /readiness"
	@echo "Verification complete."
