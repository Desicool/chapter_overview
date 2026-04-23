.PHONY: all frontend backend start dev-frontend clean

all: frontend backend

frontend:
	cd web && npm install && npm run build

backend: frontend
	go build -o chapter-overview .

start: backend
	docker compose up -d
	MOONSHOT_API_KEY=$$MOONSHOT_API_KEY ./chapter-overview serve --port 8080

dev-frontend:
	cd web && npm run dev

clean:
	rm -rf web/dist web/node_modules chapter-overview data/ tasks.db
