# Linux deployment preparation

Target confirmed by the user: market.ness.cx on an existing Debian- or Arch-based Linux server. Both nodes are reported running and synced, with NESS at http://127.0.0.1:6660 and Emercoin RPC at http://127.0.0.1:6662 on that server. Exact distribution, CPU architecture and service wallet selection are not known. Nothing has been installed or changed on that server.

## Choose the executable

Run `uname -m` on the server. Use the `linux-amd64` package for `x86_64`, or `linux-arm64` for `aarch64`/`arm64`. The binaries are built with CGO disabled; they do not require Go, Node, npm, PHP or a separate frontend server at runtime. Browser assets are embedded. The two blockchain nodes remain necessary infrastructure.

Extract the selected archive into a new directory. For public hosting start from `config.market.example.json` (under deploy/ in the source package); it sets https://market.ness.cx, secure cookies and /var/lib/ness-unified. For local testing use `config.example.json` instead. Copy the chosen template to a private `config.json`, then configure:

- NESS node URL, service wallet ID and any required local credentials.
- Emercoin node URL, wallet name and local RPC credentials.
- Explicitly chosen faucet rates and registration prices. The example's amounts were inherited from the supplied archive and are not newly approved rates.
- A fresh, private data directory. Do not point this fresh-start build at either old site's state.

Run `./ness-unified ./config.json`. Default listener: `127.0.0.1:8787`. It serves the entire application, including faucet and NVS. All service state is stored in `data_dir`; preserve its signing seed and journal between restarts. SIGTERM shuts down in-flight requests before releasing the directory lock.

## Optional systemd service

The included `ness-unified.service` is a template for a systemd host; confirm the server uses systemd before installing it. It expects:

- A dedicated local `ness-unified` user and group.
- Executable `/opt/ness-unified/ness-unified`.
- Private configuration `/etc/ness-unified/config.json`, readable by that service user.
- Configuration `data_dir` set to `/var/lib/ness-unified`; systemd creates that state directory.

Do not enable the service until these paths and settings exist. No installation script is included or implied. Keep the listener and node RPC endpoints private. Configure the existing reverse proxy for https://market.ness.cx; the production template sets this exact public_origin and secure_cookies true. DNS and certificates have not been changed.

The process refuses to start if `server.lock` remains after a forced kill. Never remove a lock while an instance may still be running. Normal SIGTERM shutdown removes it. The journal is append-only; use one instance per data directory.

## Validation boundary

Cross-compilation checks the build for both architectures. The Linux executables have not been run on the target server. Local browser checks and fake-RPC integration tests are documented in `RECOVERY-VALIDATION.md`. No live payout, name registration or desktop-wallet import has been tested. Original NVS update/renewal parity remains outstanding.
