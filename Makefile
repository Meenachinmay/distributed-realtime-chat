run:
	docker-compose down && docker-compose up --build -d
rt:
	go run loadtest/main.go