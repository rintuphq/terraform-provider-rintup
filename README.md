# Terraform Provider for RintUp

Manage [RintUp](https://rintup.com) uptime monitoring as code: monitors (HTTP, TCP, DNS, ping, SSL,
domain, page speed, heartbeat), contact groups and alert channels, maintenance windows and status pages.

- Registry address: `rintuphq/rintup` ([registry.terraform.io](https://registry.terraform.io/providers/rintuphq/rintup); OpenTofu reads the same GitHub releases)
- Requires **Terraform 1.11 or newer** (or an OpenTofu release with write-only attribute support): secrets use write-only arguments (see [Secrets](#secrets))
- Built with the HashiCorp [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework); MPL-2.0

## Quick start

1. Create an API key in the RintUp panel at https://rintup.com: **Settings > API keys** (`rup_live_...`).
   A key belongs to one workspace; everything Terraform creates lands in that workspace.
2. Configure:

```hcl
terraform {
  required_version = ">= 1.11"
  required_providers {
    rintup = { source = "rintuphq/rintup" }
  }
}

provider "rintup" {} # reads RINTUP_API_KEY (and optionally RINTUP_BASE_URL)

resource "rintup_monitor" "api" {
  type = "http"
  http = { url = "https://api.example.com/health" }
}
```

3. `export RINTUP_API_KEY=rup_live_...` then `terraform init && terraform apply`.

Only `type` and the target are required. Everything else (name, interval, locations, confirmations...)
gets the server default, which is read back into state, so the next `terraform plan` shows no changes.

A full example (API health + SSL certificate + Slack channel + status page) is in
[`examples/complete`](examples/complete/main.tf); every resource has an example under `examples/` and
reference documentation under [`docs/`](docs/index.md).

| Resource / data source | What it is |
| -- | -- |
| `rintup_monitor` | A check; one block per type: `http`, `tcp`, `dns`, `ping`, `ssl`, `domain`, `pagespeed`, `push` |
| `rintup_contact_group` | Who is alerted; monitors list groups in `contact_group_ids` |
| `rintup_alert_channel` | E-mail, SMS, webhook, Slack, Discord, Telegram, Teams, PagerDuty, Opsgenie in a group |
| `rintup_maintenance_window` | One-off or recurring (daily/weekly/monthly) silence |
| `rintup_status_page` | Public status page with its `components` list |
| data `rintup_locations` | Probe locations and their fixed egress IPs (for firewalls) |
| data `rintup_monitor` | Find an existing monitor by exact name |

## OpenTofu

The same source address works with OpenTofu:

```hcl
terraform {
  required_providers {
    rintup = { source = "rintuphq/rintup" }
  }
}
```

`tofu init` resolves `rintuphq/rintup` through the OpenTofu Registry, which serves the signed
release assets published on this repository's GitHub releases.

## Provider settings

| Argument | Environment | Default |
| -- | -- | -- |
| `api_key` (sensitive) | `RINTUP_API_KEY` | none (required) |
| `base_url` | `RINTUP_BASE_URL` | `https://rintup.com/api/v1` |

The client retries `429 Too Many Requests` (honouring `Retry-After`) and, for GET/PATCH/PUT/DELETE,
`500/502/503/504` with exponential backoff (1 s, 2 s, 4 s... up to 30 s, 5 retries). POST is not
retried on 5xx because the API has no idempotency key yet. Logs (`TF_LOG=DEBUG`) contain only
method, path and status, never headers or bodies.

## Secrets

The API stores secrets encrypted and only returns them masked (`••••`), so Terraform could never
compare them. They are therefore **write-only arguments** (Terraform 1.11+): the value is sent to the
API but never written to the plan or the state file. Each one has a `_version` companion that
Terraform does store; **change the version to send a new value**.

| Resource | Write-only argument | Version |
| -- | -- | -- |
| `rintup_monitor` (`http` block) | `headers_wo` (map), `body_wo`, `basic_auth_password_wo` | `headers_wo_version`, `body_wo_version`, `basic_auth_password_wo_version` |
| `rintup_alert_channel` | `secret_wo` (webhook signing key, Telegram bot token, PagerDuty routing key, Opsgenie key) | `secret_wo_version` |
| `rintup_alert_channel` | `target_wo` (Slack/Discord/Teams incoming webhook URL) | `target_wo_version` |
| `rintup_status_page` | `password_wo` | `password_wo_version` |

```hcl
variable "api_token" {
  type      = string
  sensitive = true
}

resource "rintup_monitor" "api" {
  type = "http"
  http = {
    url                = "https://api.example.com/health"
    headers_wo         = { Authorization = "Bearer ${var.api_token}" }
    headers_wo_version = "2026-10-30" # bump after rotating the token
  }
}
```

Rules:

- On create every configured secret is sent. On update a secret is sent only when its version changed.
- Removing a secret **and** changing (or removing) its version deletes the stored secret
  (`headers_wo` removed: all headers are removed; `password_wo` removed: the page is public again).
- The API cannot update an alert channel, so a new `secret_wo_version` / `target_wo_version`
  recreates the channel.
- A webhook channel without `secret_wo` gets a server-generated signing key that the API returns once;
  it is kept in the sensitive `generated_secret` attribute. Set `secret_wo` if the state must hold no secret.
- `push_url` (heartbeat monitors) contains the push token and is a sensitive attribute.

## Drift, deletion and import

- Every refresh reads the object from the API. A configured field changed in the panel appears in
  `terraform plan` and the next apply restores it. Fields you do not set follow the server
  (e.g. an interval changed in the panel is adopted when `interval_seconds` is not configured).
- An object deleted in the panel (API `404`) is removed from state and planned for creation again;
  `terraform destroy` treats `404` as already deleted.
- All resources support `terraform import` by id. Alert channels take `<contact_group_id>/<channel_id>`
  or just `<channel_id>`. The API never returns secrets (nor a channel's target and locale), so after an
  import the first apply sends the configured monitor/status-page secrets once (their versions were
  unknown) and adopts a channel's target/locale from the configuration without recreating it.

## Known API limits

- `GET /maintenance-windows/{id}` does not exist: reading a window scans the (paged) list.
- Channels have no `GET`/`PATCH` by id: reads go through the contact group; every change recreates the channel.
- The domain monitor reduces names to the registered domain: write `example.com`, not `www.example.com`.

## Develop locally

Requirements: Go (version in `go.mod`), Terraform 1.11+.

```sh
go build ./... && go vet ./... && go test ./...
```

`go test ./...` runs the client tests and the provider tests; the latter drive the real Terraform
CLI (found on `PATH` or via `TF_ACC_TERRAFORM_PATH`) against an in-memory fake RintUp API (CRUD,
pagination, 429 retries, 404 drift, import, and that no secret ends up in state). No network needed.

Try the provider with a local build, without touching `~/.terraformrc`, through a temporary CLI config:

```sh
go build -o "$PWD/bin/terraform-provider-rintup" .      # .exe on Windows
cat > /tmp/rintup-dev.tfrc <<EOF
provider_installation {
  dev_overrides {
    "rintuphq/rintup" = "$PWD/bin"
  }
  direct {}
}
EOF
export TF_CLI_CONFIG_FILE=/tmp/rintup-dev.tfrc
cd examples/complete
terraform validate            # no "terraform init" needed with dev_overrides
RINTUP_API_KEY=rup_live_... terraform plan
```

Acceptance tests create real objects in a workspace; use a dedicated test workspace:

```sh
TF_ACC=1 RINTUP_API_KEY=rup_live_... RINTUP_BASE_URL=https://rintup.com/api/v1 \
  go test ./internal/provider -run TestAcc -v
```

Documentation in `docs/` (Terraform Registry layout) is generated from the schema descriptions and `examples/`:

```sh
go generate ./...   # runs tfplugindocs generate
```

## Continuous integration

- `.github/workflows/test.yml` runs `go build`, `go vet`, `go test ./...` and checks that `docs/` is
  up to date on every push and pull request. When the `RINTUP_API_KEY` secret is set, a second job
  runs the acceptance tests (`TF_ACC=1`) against a real workspace; without the secret it is skipped.
- `.github/workflows/acceptance.yml` runs the acceptance tests nightly, also only when the secret exists.

## Release (maintainers)

Pushing a tag `vX.Y.Z` runs `.github/workflows/release.yml`: tests first, then GoReleaser builds
every platform, writes `terraform-provider-rintup_X.Y.Z_SHA256SUMS`, signs it with GPG
(`..._SHA256SUMS.sig`) and attaches the zips, the sums, the signature and the registry manifest to a
GitHub release. The Terraform Registry picks up new releases through its webhook.

One-time setup:

1. Create a GPG key used only for releases (RSA or ECC):
   `gpg --full-generate-key`, then note the fingerprint.
2. Repository secrets (Settings > Secrets and variables > Actions):
   - `GPG_PRIVATE_KEY`: output of `gpg --armor --export-secret-keys <fingerprint>`
   - `PASSPHRASE`: the key's passphrase
3. In the Terraform Registry, sign in with a GitHub account that is a member of the `rintuphq`
   organization, add the ASCII-armoured public key (`gpg --armor --export <fingerprint>`) under the
   namespace's signing keys, then **Publish > Provider** and select `rintuphq/terraform-provider-rintup`.
4. Optional, for acceptance tests: secret `RINTUP_API_KEY` (key of a dedicated test workspace) and
   repository variable `RINTUP_BASE_URL`.

Release: `git tag v0.1.0 && git push origin v0.1.0`.

## Security

See [SECURITY.md](SECURITY.md). Report vulnerabilities privately to security@rintup.com.

## License

[MPL-2.0](LICENSE)
