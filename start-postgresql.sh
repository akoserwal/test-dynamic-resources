set -e

DOCKER=$(command -v podman || command -v docker)
if [ -z "$DOCKER" ]; then
  echo "Neither podman nor docker is installed. Exiting."
  exit 1
fi

$DOCKER network create postgres-net || true

$DOCKER run \
  --name postgres-datastore \
  --net postgres-net \
  --restart=always \
  -e POSTGRES_PASSWORD=$(cat secrets/db.password) \
  -e POSTGRES_USER=$(cat secrets/db.user) \
  -e POSTGRES_DB=$(cat secrets/db.name) \
  -p 5432:5432 \
  -d postgres:latest