run:
	docker-compose down && docker-compose up --build -d
rst:
	go run cmd/server/main.go
rct:
	go run cmd/loadtest/main.go
stop:
	docker-compose down