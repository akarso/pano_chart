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

If you want I can extend this runbook with deployment manifests (Kubernetes YAML), recommended CI job snippets for release signing, or a Fastlane configuration template for Android/iOS. Tell me which you prefer and I will add it.
