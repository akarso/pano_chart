# Frontend Git Hooks

This repository includes a tracked pre-commit hook for the frontend that
automatically formats Dart code in `frontend/` and stages any formatting
changes before a commit.

Files added:

- `frontend/.githooks/pre-commit` — the pre-commit hook script (tracked)
- `frontend/scripts/install-hooks.sh` — installer script to enable hooks in your local repo

How to enable hooks (run once locally):

```bash
cd /path/to/pano_chart
bash frontend/scripts/install-hooks.sh
```

Notes:

- The hook runs `dart format .` inside `frontend/`. Ensure `dart` (or `flutter`)
  is installed and on your `PATH` so formatting works.
- If `dart` is not available the hook will warn and continue — formatting will be skipped.
- Hooks are not automatically enabled for other clones; each developer should run the installer.

To revert the change:

```bash
git config --unset core.hooksPath
```
