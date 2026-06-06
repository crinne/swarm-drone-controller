# swarm-drone-controller

Small Go controller for Swarm GCS.

It exposes:

- `GET /health`
- `GET /ready`
- `POST /spawn`

`POST /spawn` validates `X-Spawn-Password`, finds the first available drone ID,
and creates a long-running drone Pod that uses the configured simulator image.

## Configuration

Required environment variables:

- `NAMESPACE`: Kubernetes namespace to manage
- `DRONE_IMAGE`: simulator image used for spawned drones
- `SPAWN_PASSWORD`: shared spawn password
- `ALLOWED_ORIGIN`: CORS origin, usually `https://swarmgcs.dev`

## Local Checks

```bash
go test ./...
go vet ./...
docker build -t drone-controller:test .
```
