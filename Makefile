ifneq (,$(wildcard ./.env.test))
    include .env.test
    export
endif

.PHONY: test-integration
test-integration:
	@echo "Spinning up disposable test database..."
	docker-compose -f docker-compose.test.yml up -d test_db
	@echo "Waiting for test database..."
	@docker-compose -f docker-compose.test.yml exec -T test_db pg_isready -U test_user -t 10 || (echo "Still waiting..." && timeout /t 5)
	@echo "Applying migrations..."
	npx prisma migrate deploy
	@echo "Running Go tests..."
	go test ./internals/repository/... -v
	@echo "Cleaning up..."
	docker-compose -f docker-compose.test.yml stop test_db