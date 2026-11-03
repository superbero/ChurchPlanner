echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
