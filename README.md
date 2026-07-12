# Foundry Package Release

GitHub Action that publishes a Foundry VTT package release via the
[Package Release API](https://foundryvtt.com/article/package-release-api/).

## Usage

```yaml
- name: Publish release
  uses: neovatar/foundry-package-release@v1
  with:
    token: ${{ secrets.FVTTP_TOKEN }}
    id: example-module
    version: 1.0.0
    manifest: https://example.com/releases/1.0.0/module.json
    notes: https://example.com/releases/1.0.0
    compat_min: '10.312'
    compat_verified: '12'
    compat_max: '12.999'
    dry_run: 'false'
```

## Inputs

| Name              | Required | Description                                                        |
|-------------------|----------|----------------------------------------------------------------------|
| `token`           | Yes      | Package Release Token, from the package page on foundryvtt.com       |
| `id`              | Yes      | The package ID, as found in its manifest                             |
| `version`         | Yes      | The semantic version of this release (e.g. `1.0.0`)                  |
| `manifest`        | Yes      | URL to the manifest for this specific release (not the latest/rolling manifest) |
| `notes`           | No       | URL to the release notes for this release                            |
| `compat_min`      | Yes      | Minimum Foundry VTT version this release is compatible with          |
| `compat_verified` | Yes      | Most recent Foundry VTT version this release has been verified against |
| `compat_max`      | No       | Foundry VTT version at which this release is no longer compatible    |
| `dry_run`         | No       | Validate the release without publishing it. Defaults to `true`       |

## Outputs

| Name      | Description                                                        |
|-----------|---------------------------------------------------------------------|
| `status`  | The status returned by the Foundry package release API (`success` or `error`) |
| `page`    | URL of the package edit page returned by the API                    |
| `message` | Message returned by the API (populated on dry runs)                 |

## Local testing

Run the unit tests, which cover the API request/response handling against a
fake server (no network access or real token required):

```sh
go test ./...
```

To exercise the real binary against the live API, set the same
`INPUT_*`/`GITHUB_OUTPUT` environment variables GitHub Actions would set,
then run it directly:

```sh
GITHUB_OUTPUT=/tmp/gh-output \
INPUT_TOKEN=fvttp_xxx \
INPUT_ID=example-module \
INPUT_VERSION=1.0.0 \
INPUT_MANIFEST=https://example.com/releases/1.0.0/module.json \
INPUT_NOTES=https://example.com/releases/1.0.0 \
INPUT_COMPAT_MIN=10.312 \
INPUT_COMPAT_VERIFIED=12 \
INPUT_COMPAT_MAX=12.999 \
INPUT_DRY_RUN=true \
go run .
```

Notes:

- `GITHUB_OUTPUT` must point to a writable file; it doesn't need to pre-exist.
- Check the results afterwards with `cat /tmp/gh-output`.
