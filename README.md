# nodexa-cli

CLI for building and pushing container images to [nodexa-registry](https://github.com/nodexa-os/nodexa-registry).

## Install

```bash
go install github.com/nodexa-os/nodexa-cli/cmd/nodex@latest
```

Or build from source:

```bash
go build -o nodex ./cmd/nodex/
```

## Usage

Create a `nodexa.yml` in your project root:

```yaml
services:
  - name: web
    dockerfile: Dockerfile
    context: .
  - name: worker
    dockerfile: worker/Dockerfile
    context: worker/
```

Push a release:

```bash
nodex push \
  --registry nodexa.elzora.tech \
  --registry-url https://nodexa.elzora.tech/registry \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token>
```

### What `nodex push` does

1. Parses `nodexa.yml` (or `--file <path>`)
2. Reserves a release on nodexa-registry → gets a revision number + image refs
3. `docker login` to the registry with your API token
4. For each service: `docker build` → `docker push` → captures digest
5. Completes the release with digests (or marks it failed on error)

### Flags

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `-f, --file` | — | `nodexa.yml` | Path to manifest |
| `--fleet-id` | `NODEXA_FLEET_ID` | — | Fleet ID (required) |
| `--registry` | `NODEXA_REGISTRY_HOST` | `localhost:5000` | Registry host for docker login |
| `--registry-url` | `NODEXA_REGISTRY_URL` | `http://localhost:8000` | nodexa-registry API URL |
| `--token` | `NODEXA_API_TOKEN` | — | API token (required) |

## `nodexa.yml` Spec

```yaml
services:
  - name: <string>         # required — matches Deployment.services[].name
    dockerfile: <string>   # optional — default: "Dockerfile"
    context: <string>      # optional — default: "."
```

## Project Structure

```
nodexa-cli/
├── cmd/nodex/main.go           # Cobra root + push command
└── internal/
    ├── build/build.go           # docker build/push/login wrappers
    ├── client/client.go         # nodexa-registry HTTP client
    └── manifest/manifest.go     # nodexa.yml parser
```
