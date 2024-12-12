set -e

DOCKER=$(command -v podman || command -v docker)
if [ -z "$DOCKER" ]; then
  echo "Neither podman nor docker is installed. Exiting."
  exit 1
fi

$DOCKER stop postgres-datastore || true
$DOCKER rm postgres-datastore || true
$DOCKER network rm postgres-net || true

echo "Teardown completed, All resources have been cleaned up."
