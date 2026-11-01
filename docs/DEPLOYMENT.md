# Automatic web deployment

Pushes to `main` and manual runs of **Deploy web** use a standard GitHub-hosted
Linux runner. Pull requests do not run this deployment workflow. No runner,
SSH key, GitHub write token or registry credential is installed on the server.

The workflow runs frontend tests/build, backend tests with the race detector and
go vet, then builds the production Docker image. It publishes a public GitHub
release named `web-<full commit SHA>` containing `web-image.tar.gz` and a checksum
manifest. These files contain compiled application code only; no runtime data or
credentials are part of the image. The manifest ties the image to the commit and
workflow run. Build releases are immutable in the normal workflow: reruns reuse
an existing release instead of replacing its files.

The administrator-managed updater checks releases about every two minutes. It
accepts only an Actions release for the current `main`, validates its workflow
origin, checksum and image revision, and loads the ready-made image. It stops
only DW Practice, takes a verified SQLite backup including WAL contents, starts
the new image and checks both container health and the public HTTPS endpoint.
The existing database, email settings, session secret, routing and other services
remain in place. On failure it restores the previous image and database snapshot,
while preserving the failed database files for diagnosis. A failed commit is not
retried automatically; an administrator must request a retry or publish a fix.

`https://dw.domyshev.com/deployment.json` exposes the running commit. After that
commit and the health check are confirmed externally, the GitHub workflow creates
`deploy-web-<UTC timestamp>-<short SHA>` pointing to the exact deployed commit.
A `web-*` build release alone does not claim a successful production deployment.
Check the Actions run summary and the deployment tag for the final result.

The pipeline serializes main deployments and skips superseded commits. Failures
appear in GitHub Actions and in the server updater's systemd journal. Server
configuration, operation logs, backup/recovery procedures and updater tests are
maintained in the private infrastructure repository. This automation updates the
web container only; iPhone signing, installation and distribution remain manual.
