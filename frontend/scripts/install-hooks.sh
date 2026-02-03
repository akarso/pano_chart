#!/usr/bin/env bash
set -euo pipefail

echo "Installing git hooks for frontend..."
git config core.hooksPath frontend/.githooks
echo "Done. Git hooks path set to 'frontend/.githooks'"
echo "If you need to revert: git config --unset core.hooksPath"
