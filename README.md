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

### 4. Using Pre-Built Images (`docker-compose.yml` or `nodexa.yml`)

If a service in your `docker-compose.yml` or `nodexa.yml` points to an already-built image (e.g., published by CI/CD to GitHub Container Registry or Docker Hub) without a `build` block:

```yaml
services:
  firmware:
    image: ghcr.io/my-org/firmware:latest
  nginx:
    build: ./nginx
```

`nodex push` automatically detects `firmware` as a pre-built image:
- It **pulls** `ghcr.io/my-org/firmware:latest` (targeting `--platform` if specified)
- **Tags** it for the Nodexa release repository
- **Pushes** it to `nodexa.elzora.tech` without rebuilding
- Builds only services that declare an explicit `build` block (`nginx`)

---

## External Registry Authentication (`registry.yml`)

When pulling pre-built images from private registries (e.g. private repositories on `ghcr.io`, Docker Hub, AWS ECR, GitLab Container Registry), provide your credentials in a `registry.yml` file.

### Supported File Names
`nodex push` automatically detects any of the following files in the project directory:
- `registry.yml` / `registry.yaml`
- `registries.yml` / `registries.yaml`
- Or specify an explicit path with `-r` / `--registry-file <path>`.

### `registry.yml` Formats

**Format 1: Key-Value by Registry Host (Recommended)**
```yaml
ghcr.io:
  username: my-github-username
  password: ghp_personalAccessTokenWithReadPackages

docker.io:
  username: my-docker-username
  password: dckr_pat_secretToken
```

**Format 2: Wrapped under `registries` or `auths`**
```yaml
registries:
  ghcr.io:
    username: my-github-username
    password: ghp_personalAccessTokenWithReadPackages
```

**Format 3: List**
```yaml
- registry: ghcr.io
  username: my-github-username
  password: ghp_personalAccessTokenWithReadPackages
```

> **Security Note**: Never commit `registry.yml` to version control. Add it to your `.gitignore`:
> ```bash
> echo "registry*.yml" >> .gitignore
> ```

### Push Command Examples with Registry Authentication

**Automatic Detection** (if `registry.yml` is in the current directory):
```bash
nodex push \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token> \
  --platform linux/arm/v7 \
  -f docker-compose.yml
```

**Explicit Registry File** (using `-r` or `--registry-file`):
```bash
nodex push \
  --fleet-id <fleet-uuid> \
  --token <nodexa-api-token> \
  --platform linux/arm/v7 \
  -f docker-compose.yml \
  -r registry.yml
```

**Using Custom Path**:
```bash
nodex push \
  --fleet-id 6aa7090a7f1a3400237fa78c \
  --token <nodexa-api-token> \
  --platform linux/arm/v7 \
  --registry-file ~/.config/nodexa/registry.yml \
  -f docker-compose.yml
```

Before pulling images, `nodex push` reads the file, logs into each external registry (e.g. `ghcr.io`), pulls pre-built images with authentication, tags them for Nodexa, and pushes them to `nodexa.elzora.tech`.

---

## Target Architecture / Platform (`--platform`)

To build or pull images targeting specific hardware (e.g., BeagleBone ARMv7 32-bit, Raspberry Pi 4 ARM64, x86_64 gateway):

```bash
# BeagleBone (ARMv7 32-bit)
nodex push --fleet-id <id> --token <token> --platform linux/arm/v7

# Multi-platform build & push (ARM64 + ARMv7)
nodex push --fleet-id <id> --token <token> --platform linux/arm64,linux/arm/v7
```

You can also set the `NODEXA_PLATFORM` environment variable or specify `platform:` inside `nodexa.yml` or `docker-compose.yml`.

---

### What `nodex push` does

1. Loads external registry credentials (`registry.yml`) and logs into external registries if present
2. Resolves service configuration:
   - Reads `nodexa.yml` or `nodexa.yaml` if present
   - Or auto-detects from `docker-compose.yml` / `compose.yml`
   - Or auto-detects from `Dockerfile` in the current repository
   - Or uses explicit CLI flags (`--service`, `--dockerfile`, `--context`)
3. Reserves a release on nodexa-registry → gets a revision number + image refs
4. `docker login` to the Nodexa registry (`nodexa.elzora.tech`) with your API token
5. For each service:
   - **Pre-built image**: `docker pull` → `docker tag` → `docker push` → captures digest
   - **Source build**: `docker build` (or `buildx` for multi-platform) → `docker push` → captures digest
6. Completes the release with digests (or marks it failed on error)

### Flags

| Flag | Short | Env var | Default | Description |
|------|-------|---------|---------|-------------|
| `--fleet-id` | — | `NODEXA_FLEET_ID` | — | Fleet ID (required) |
| `--token` | — | `NODEXA_API_TOKEN` | — | API token (required) |
| `-p, --platform` | `-p` | `NODEXA_PLATFORM` | — | Target platform (e.g. `linux/arm/v7`, `linux/arm64`) |
| `-r, --registry-file` | `-r` | — | — | Path to `registry.yml` credentials file (auto-detected) |
| `-f, --file` | `-f` | — | `nodexa.yml` | Path to manifest or compose file |
| `-c, --compose-file` | `-c` | — | — | Path to `docker-compose.yml` file |
| `-s, --service` | `-s` | — | — | Service name override |
| `--dockerfile` | — | — | `Dockerfile` | Path to Dockerfile |
| `--context` | — | — | `.` | Docker build context directory |

> **Registry Target**: The CLI natively targets the Nodexa cloud registry (`nodexa.elzora.tech`). End users do not need to specify registry hosts or URLs.

---

## `nodexa.yml` Spec

```yaml
platform: linux/arm/v7    # optional — global default platform for all services

services:
  # Option A: Build from source
  - name: web              # required — matches Deployment.services[].name
    dockerfile: Dockerfile # optional — default: "Dockerfile"
    context: .             # optional — default: "."
    platform: linux/arm/v7 # optional — override platform per-service

  # Option B: Use pre-built image
  - name: firmware
    image: ghcr.io/my-org/firmware:v1.2.0
```

## Project Structure

```
nodexa-cli/
├── cmd/nodex/main.go           # Cobra root + push command
└── internal/
    ├── build/build.go           # docker build/push/pull/login wrappers
    ├── client/client.go         # nodexa-registry HTTP client
    ├── manifest/manifest.go     # nodexa.yml & compose parser + repo detector
    └── registry/registry.go     # registry.yml credentials parser
```
