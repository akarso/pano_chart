#!/usr/bin/env bash
set -euo pipefail

echo "Installing git hooks for frontend..."
git config core.hooksPath frontend/.githooks
# Ensure the tracked pre-commit hook is executable so Git will run it.
if [ -f frontend/.githooks/pre-commit ]; then
	chmod +x frontend/.githooks/pre-commit || true
fi
echo "Done. Git hooks path set to 'frontend/.githooks'"
echo "If you need to revert: git config --unset core.hooksPath"
