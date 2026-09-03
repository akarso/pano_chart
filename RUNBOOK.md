# RUNBOOK — Deploy & Configure Backend and Frontend

This runbook collects the standard steps to build, test, deploy, and configure the `pano_chart` backend and frontend (Android & iOS). It is intentionally pragmatic — follow these steps in order and adapt them to your infra (Kubernetes, VM, CI provider).

**Quick index**
- **Prerequisites**
- **Backend: local run, build, Docker, deploy**
- **Frontend: local run, tests, build Android, build iOS**
- **CI / Release checklist**
- **Monitoring & Rollback**
- **Troubleshooting**

---

**Prerequisites**

- A machine with required toolchains installed (macOS recommended for iOS builds):
  - Go 1.22+ (backend)
  - Docker (when producing images)
  - Flutter SDK (stable channel) + Dart (frontend)
  - Xcode and CocoaPods (for iOS builds)
  - Android SDK & Java + keystore (for Android builds)
- GitHub access for pushing and triggering CI
- CI runners with access to secrets (API credentials, signing keys)

All terminal commands below assume `zsh` and the repository root.

---

**Backend — Local development**

- Install Go and dependencies, then run tests and lint:

```bash
cd backend
go test ./...        # run unit tests
gofmt -s -w .        # format
golangci-lint run ./...  # static analysis (if installed)
go build ./...        # compile
```

- Run the service locally (example using env vars):

```bash
export PC_PORT=8080
export PC_DATABASE_URL="postgres://user:pass@localhost:5432/pano"
cd backend/cmd/server
go run .
```

**Backend — Docker image**

- Build and test a Docker image locally:

```bash
cd backend
docker build -t pano_chart_backend:local .
docker run --rm -p 8080:8080 \
  -e PC_DATABASE_URL='postgres://user:pass@db:5432/pano' \
  pano_chart_backend:local
```

**Backend — Database migrations**

- Apply DB migrations (adapt to your migration tool):

```bash
cd backend
# example if using golang-migrate
migrate -path migrations -database "$PC_DATABASE_URL" up
```

**Backend — Deployment (Kubernetes example)**

- Build image, push to registry, update k8s manifests, and apply:

```bash
# build & push
docker build -t registry.example.com/pano_chart/backend:v1.2.3 -f backend/Dockerfile backend
docker push registry.example.com/pano_chart/backend:v1.2.3

# update k8s Deployment image and apply
kubectl set image deployment/pano-backend pano-backend=registry.example.com/pano_chart/backend:v1.2.3
kubectl rollout status deployment/pano-backend
```

Environment configuration (common):
- `PC_DATABASE_URL` — Postgres connection string
- `PC_REDIS_URL` — Redis (optional)
- `PC_PORT` — listen port
- `PC_TLS_CERT` / `PC_TLS_KEY` — if embedding TLS certs (prefer ingress termination)

Secrets: keep signing keys, DB passwords, and any API keys in your secrets manager (GitHub Actions secrets, Vault, or k8s Secrets). Never commit credentials.

---

**Frontend — Local development & tests**

- Install Flutter stable and ensure `flutter`/`dart` are on PATH.

```bash
cd frontend
flutter pub get
flutter analyze
flutter test
```

- Install git hooks (optional) to enforce formatting:

```bash
bash frontend/scripts/install-hooks.sh
```

**Frontend — App configuration**

- The app reads `AppConfig(apiBaseUrl: ..., flavor: ...)` via constructor injection.
- For development set the base URL to your backend dev host (e.g. `https://api.dev.example`). Do not hard-code production URLs in code.

**Frontend — Build Android (AAB recommended)**

1. Generate/apply release keystore and add to CI secrets. Example local build:

```bash
cd frontend
flutter build appbundle --release --target-platform android-arm,android-arm64
# Produces build/app/outputs/bundle/release/app-release.aab
```

2. Signing: configure `android/key.properties` or CI to sign the artifact. Upload `.aab` to Google Play Console (internal testing → production as needed).

**Frontend — Build iOS (Archive)**

1. On macOS with Xcode and CocoaPods:

```bash
cd frontend
flutter build ios --release
# Open Xcode to archive, or use `xcodebuild` / Fastlane to archive and upload
```

2. Provisioning & Signing: Ensure you have an App ID, provisioning profile, and certificate. Use Xcode organizer or Fastlane match to produce signed archives and upload to TestFlight/App Store.

**CI Notes (Frontend)**

- Our repo contains `frontend/.github/workflows/frontend-ci.yml` which:
  - checks out code
  - installs Flutter on runner
  - runs `dart format --set-exit-if-changed .`
  - runs `flutter analyze` and `flutter test`

- For release pipelines add steps to:
  - build signed Android `.aab` and upload to Play Console (use `fastlane supply` or Google Play API)
  - build signed iOS archive and upload to App Store Connect (via Fastlane `pilot`)
  - use CI secrets for signing keys and store credentials

---

**Release checklist**

- Backend
  - [ ] All tests passing locally and in CI
  - [ ] Migration plan approved (if DB changes)
  - [ ] Image built, pushed, and smoke-tested in staging
  - [ ] Health checks and readiness probes configured

- Frontend
  - [ ] All widget + unit tests pass in CI
  - [ ] Format & analysis pass
  - [ ] Signed artifacts produced and uploaded to store or staging distribution
  - [ ] Release notes prepared

---

**Monitoring & Rollback**

- Monitor:
  - Backend logs (stackdriver / CloudWatch / ELK / your logging)
  - Metrics (Prometheus/Grafana or provider monitoring)
  - Uptime (external ping/health checks)

- Rollback strategies:
  - Kubernetes: `kubectl rollout undo deployment/<name>`
  - Docker / systemd: re-deploy previously known-good image
  - Mobile: push hotfix release to stores (TestFlight/Play internal) if needed

---

**Troubleshooting**

- Backend fails to start:
  - Check `PC_DATABASE_URL` and database connectivity
  - Inspect logs for panic or missing env vars

- Frontend tests failing in CI but ok locally:
  - Ensure CI runner has same Flutter channel; pin action to `subosito/flutter-action@v2` and `channel: stable`
  - Run `flutter pub get` and check `pubspec.lock` sync

- Formatting failures in CI:
  - Run `dart format .` locally and commit changes
  - Installer script `frontend/scripts/install-hooks.sh` enables local pre-commit formatting hooks

---

**Push Notifications — Debian Server Setup**

This section covers everything needed on the Debian backend to send push notifications to mobile clients via Firebase Cloud Messaging (FCM). FCM is the unified channel for both Android and iOS.

**Overview**

```
┌──────────────┐    FCM HTTP v1 API    ┌───────────────┐
│  Go backend  │ ──────────────────►   │  Google FCM   │
│  (Debian)    │   Bearer token (JWT)  │  servers      │
└──────────────┘                       └───────┬───────┘
       │                                       │
       │  stores device tokens                 │ delivers push
       │  in SQLite                            ▼
       │                               ┌──────────────┐
       └───────────────────────────────►│  APNs / GCM  │
                                        │  → device    │
                                        └──────────────┘
```

**1 — System packages (Debian)**

```bash
# Minimal — the Go binary handles TLS natively, no extra libs needed.
# Just ensure ca-certificates is up to date for outbound HTTPS to FCM:
sudo apt update
sudo apt install -y ca-certificates
sudo update-ca-certificates
```

No other system-level dependencies. The Go binary statically links everything.
If you compile on the server (not recommended — prefer cross-compile), also install:

```bash
sudo apt install -y golang-go gcc   # only if building on-server
```

**2 — Firebase project setup (one-time, in browser)**

1. Go to [Firebase Console](https://console.firebase.google.com/)
2. Create or select your project
3. Enable **Cloud Messaging** (Project Settings → Cloud Messaging tab)
4. Go to **Project Settings → Service Accounts**
5. Click **"Generate new private key"** → downloads a JSON file
6. This JSON file is your `GOOGLE_APPLICATION_CREDENTIALS` — it contains the service account email, private key, and project ID

**3 — Deploy credentials to server**

```bash
# Copy the service account JSON to the server (secure transfer)
scp firebase-service-account.json user@your-server:/etc/pano_chart/

# Lock down permissions
sudo chown root:pano_chart /etc/pano_chart/firebase-service-account.json
sudo chmod 640 /etc/pano_chart/firebase-service-account.json
```

**4 — Environment variables**

Add to your service environment (systemd unit, .env, or export):

```bash
# Path to the Firebase service account JSON
GOOGLE_APPLICATION_CREDENTIALS=/etc/pano_chart/firebase-service-account.json

# Firebase project ID (also in the JSON, but explicit is clearer)
FCM_PROJECT_ID=your-firebase-project-id
```

These are read by the Go backend at startup.

**5 — Go dependencies**

Add the Firebase Admin SDK or use the FCM HTTP v1 API directly:

```bash
cd backend

# Option A: Firebase Admin SDK (higher-level, handles token refresh)
go get firebase.google.com/go/v4
go get google.golang.org/api/option

# Option B: Raw HTTP v1 (lighter, fewer deps — uses google.golang.org/api for auth)
go get golang.org/x/oauth2/google
go get google.golang.org/api/option
```

Option A is recommended — it handles OAuth2 token refresh and provides a typed `messaging.Client`.

**6 — SQLite device token table**

Device tokens are stored alongside existing payment data in SQLite. Migration:

```sql
CREATE TABLE IF NOT EXISTS device_tokens (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id   TEXT    NOT NULL UNIQUE,   -- stable device identifier
    fcm_token   TEXT    NOT NULL,          -- FCM registration token
    platform    TEXT    NOT NULL,          -- "android" | "ios"
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_device_tokens_fcm ON device_tokens(fcm_token);
```

Path controlled by existing env var `PAYMENT_DB_PATH` (default: `./payment.sqlite`).

**7 — Flutter client setup**

On the Flutter side (not Debian, but needed for the full flow):

```bash
cd frontend
flutter pub add firebase_core firebase_messaging
```

Then:
- Add `google-services.json` to `android/app/`
- Add `GoogleService-Info.plist` to `ios/Runner/`
- Register for push in app bootstrap, send the FCM token to backend via `POST /api/device/register`

**8 — iOS APNs certificate (one-time, Apple Developer Portal)**

FCM relays to APNs for iOS. You need one of:

- **APNs Auth Key** (recommended, no expiry):
  1. Apple Developer → Keys → Create key with "Apple Push Notifications service (APNs)"
  2. Download the `.p8` file
  3. Upload it to Firebase Console → Project Settings → Cloud Messaging → iOS → APNs Authentication Key

- **APNs Certificate** (expires annually):
  1. Generate CSR, upload to Apple Developer → Certificates → APNs
  2. Download `.cer`, convert to `.p12`
  3. Upload to Firebase Console

The APNs key lives in Firebase Console — the Debian server does not need it directly.

**9 — Firewall rules**

The Go backend needs outbound HTTPS to FCM:

```bash
# If running iptables/nftables with restrictive egress:
# Allow outbound to Google FCM endpoints
sudo iptables -A OUTPUT -p tcp --dport 443 -d fcm.googleapis.com -j ACCEPT
sudo iptables -A OUTPUT -p tcp --dport 443 -d oauth2.googleapis.com -j ACCEPT

# Most setups allow all outbound and this is not needed.
```

**10 — Systemd integration (if using systemd)**

If the backend runs as a systemd service, add the env var to the unit:

```ini
# /etc/systemd/system/pano-backend.service
[Service]
Environment=GOOGLE_APPLICATION_CREDENTIALS=/etc/pano_chart/firebase-service-account.json
Environment=FCM_PROJECT_ID=your-firebase-project-id
# ... existing env vars (PC_REDIS_ADDR, PAYMENT_DB_PATH, etc.)
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart pano-backend
```

**11 — Verification checklist**

```bash
# 1. Credentials file exists and is readable by the service user
sudo -u pano_chart cat /etc/pano_chart/firebase-service-account.json | head -3

# 2. Outbound HTTPS works
curl -s -o /dev/null -w "%{http_code}" https://fcm.googleapis.com
# Should return 404 (endpoint exists, no valid request) — proves connectivity

# 3. SQLite table exists
sqlite3 /path/to/payment.sqlite ".tables" | grep device_tokens

# 4. Backend health
curl http://localhost:8080/health
```

**Summary — what to install / configure on Debian**

| Item | What | Where |
|------|------|-------|
| `ca-certificates` | apt package | system |
| Firebase service account JSON | credential file | `/etc/pano_chart/` |
| `GOOGLE_APPLICATION_CREDENTIALS` | env var → JSON path | systemd unit / .env |
| `FCM_PROJECT_ID` | env var | systemd unit / .env |
| `firebase.google.com/go/v4` | Go module | `go.mod` |
| `device_tokens` table | SQLite migration | `PAYMENT_DB_PATH` |
| APNs Auth Key `.p8` | uploaded to Firebase Console | Firebase Console (not on server) |
| Firewall egress to `fcm.googleapis.com` | outbound 443 | iptables (if restrictive) |

---

**Appendix — Useful commands**

- Run full backend verification locally:
```bash
cd backend
gofmt -s -w .
go test ./...
golangci-lint run ./... || true
go build ./...
```

- Run frontend verification locally:
```bash
cd frontend
flutter pub get
dart format --set-exit-if-changed .   # use without --set-exit-if-changed to auto-format
flutter analyze
flutter test
```

---

**Backend — Run on VPS (root access)**

This section describes how to deploy and run the backend on a Linux VPS where you have root access. This is suitable for simple production or staging deployments without container orchestration.

1. **Provision your VPS**
   - Use a modern Linux distribution (Ubuntu 22.04 LTS or similar recommended).
   - Ensure you have root or sudo access.

2. **Install dependencies**
   - Install Go (if building from source):
     ```bash
     sudo apt update && sudo apt install -y golang
     # Or download from https://go.dev/dl/
     ```
   - Install PostgreSQL client (if needed):
     ```bash
     sudo apt install -y postgresql-client
     ```

3. **Copy backend binary and assets**
   - Build the backend on your dev machine:
     ```bash
     cd backend
     go build -o pano_chart_server ./cmd/server
     ```
   - Copy the binary to your VPS (replace $VPS with your server IP):
     ```bash
     scp backend/pano_chart_server user@$VPS:/opt/pano_chart/
     # Or use rsync for directories
     ```

4. **Set up environment variables**
   - Create a file `/opt/pano_chart/.env` with your secrets:
     ```env
     PC_PORT=8080
     PC_DATABASE_URL=postgres://user:pass@localhost:5432/pano
     # Add any other required env vars
     ```

5. **Create a systemd service**
   - Create `/etc/systemd/system/pano_chart.service`:
     ```ini
     [Unit]
     Description=Pano Chart Backend
     After=network.target

     [Service]
     Type=simple
     WorkingDirectory=/opt/pano_chart
     EnvironmentFile=/opt/pano_chart/.env
     ExecStart=/opt/pano_chart/pano_chart_server
     Restart=on-failure
     User=root

     [Install]
     WantedBy=multi-user.target
     ```
   - Reload systemd and start the service:
     ```bash
     sudo systemctl daemon-reload
     sudo systemctl enable pano_chart
     sudo systemctl start pano_chart
     sudo systemctl status pano_chart
     ```

6. **Configure firewall (optional but recommended)**
   - Allow only required ports (e.g., 8080 for HTTP):
     ```bash
     sudo ufw allow 8080/tcp
     sudo ufw enable
     ```

7. **Logs and troubleshooting**
   - View logs with:
     ```bash
     journalctl -u pano_chart -f
     ```
   - Check for crashes, port conflicts, or missing environment variables.

8. **(Optional) Set up HTTPS**
   - Use a reverse proxy (nginx, Caddy, or Traefik) to terminate TLS and forward to your backend.
   - Or, configure the backend to serve TLS directly if supported (set `PC_TLS_CERT` and `PC_TLS_KEY`).

---
