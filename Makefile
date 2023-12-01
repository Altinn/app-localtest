.PHONY: start-localtest
start-localtest:
	docker-compose --profile localtest up -d --build

.PHONY: stop-localtest
stop-localtest:
	docker-compose --profile localtest down

.PHONY: start-localtest-new-pdf
start-localtest-new-pdf:
	docker-compose --profile localtest --profile pdf up -d --build

.PHONY: stop-localtest-new-pdf
stop-localtest-new-pdf:
	docker-compose --profile localtest --profile pdf down

.PHONY: start-localtest-multiapp
start-localtest-multiapp:
	docker-compose --profile pdf up -d --build
	dotnet run --project src/LocalTest.csproj