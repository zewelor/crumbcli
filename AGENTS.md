# CrumbCLI

Read README.md for the CLI contract, operation and verification boundaries. Keep the project a small local Linux CLI, with application code and tests in cmd/crumbcli/. Use the standard library for flags, JSON and test assertions; the optional framework defaults in upstream skills do not override this project decision.

The CLI intentionally emits selected cookie values as JSON on stdout. Keep values out of diagnostics, test reports, artifacts and public commits; use synthetic data in tests. Real browser-session reads require task authorization, and synthetic checks are not live compatibility proof. Keep the MVP extraction-only: no Vault integration, network publication or automatic persistence.

Use Red/Green for changed extraction/validation behavior. Run `just ci` and `just dist` before delivery; report any skipped boundary.

Five local skills from samber/cc-skills-golang cover style, errors, testing, CLI and credential security. Provenance is in skills-lock.json. Check local divergence before `npx skills update <skill-name> -p -y`; review files and lock metadata afterward. Keep .agents out of application formatting and dependency extraction. Author skill changes upstream rather than editing vendored copies.

Commit, push and release are separate actions; follow the current user's authorization for each. Preserve unrelated files and do not rewrite Git history autonomously.
