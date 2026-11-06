echo "git updated"
python3 scripts/git_push.py
echo "--------------------------------------"
echo "✓ build OK"
docker compose -f deploy/docker-compose.yml up -d --build
