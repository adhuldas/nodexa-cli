# nodexa-cli

CLI for building and pushing container images to [nodexa-registry](https://github.com/adhuldas/nodexa-registry).

## Install

### Quick Install (macOS & Linux)

Install directly to `/usr/local/bin` without needing Go installed:

```bash
curl -fsSL https://raw.githubusercontent.com/adhuldas/nodexa-cli/main/install.sh | sh
```

### With Go

```bash
go install github.com/adhuldas/nodexa-cli/cmd/nodex@latest
```

### Build from Source

```bash
git clone https://github.com/adhuldas/nodexa-cli.git
cd nodexa-cli
go build -o /usr/local/bin/nodex ./cmd/nodex
```

Verify the installation:
```bash
nodex --version
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
| `--token` | `NODEXA_API_TOKEN` | — | API token (required) |

> **Registry Target**: The CLI natively targets the Nodexa cloud registry (`nodexa.elzora.tech`). End users do not need to specify registry hosts or URLs.

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
