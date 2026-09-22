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

`nodex push` works out of the box in any repository with zero configuration:

### 1. Zero-Config Push (from any repo with a `Dockerfile` or `docker-compose.yml`)

Navigate to your application repository and push directly:

```bash
nodex push \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token>
```

- **`docker-compose.yml`**: Automatically detects buildable services or matches services defined in compose with local Dockerfiles.
- **`Dockerfile`**: Automatically detects the Dockerfile in the root and infers the service name from the directory or compose service.
- **Override service name**: `--service <name>` (or `-s <name>`).

### 2. Using `docker-compose.yml` Explicitly

```bash
nodex push \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token> \
  -c docker-compose.yml
```

### 3. Using a `nodexa.yml` Manifest (Multi-service releases)

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

Push using the manifest:

```bash
nodex push \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token>
```

Or pass a custom manifest path:
```bash
nodex push --fleet-id <fleet-uuid> --token <token> -f deploy/staging.yml
```

---

### What `nodex push` does

1. Resolves service configuration:
   - Reads `nodexa.yml` or `nodexa.yaml` if present
   - Or auto-detects from `docker-compose.yml` / `compose.yml`
   - Or auto-detects from `Dockerfile` in the current repository
   - Or uses explicit CLI flags (`--service`, `--dockerfile`, `--context`)
2. Reserves a release on nodexa-registry → gets a revision number + image refs
3. `docker login` to the registry with your API token
4. For each service: `docker build` → `docker push` → captures digest
5. Completes the release with digests (or marks it failed on error)

### Flags

| Flag | Short | Env var | Default | Description |
|------|-------|---------|---------|-------------|
| `--fleet-id` | — | `NODEXA_FLEET_ID` | — | Fleet ID (required) |
| `--token` | — | `NODEXA_API_TOKEN` | — | API token (required) |
| `-f, --file` | `-f` | — | `nodexa.yml` | Path to manifest or compose file |
| `-c, --compose-file` | `-c` | — | — | Path to `docker-compose.yml` file |
| `-s, --service` | `-s` | — | — | Service name override |
| `--dockerfile` | — | — | `Dockerfile` | Path to Dockerfile |
| `--context` | — | — | `.` | Docker build context directory |

> **Registry Target**: The CLI natively targets the Nodexa cloud registry (`nodexa.elzora.tech`). End users do not need to specify registry hosts or URLs.

---

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
    └── manifest/manifest.go     # nodexa.yml & compose parser + repo detector
```
