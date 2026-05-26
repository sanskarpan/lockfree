# Security Policy

## Supported Versions

Only the latest `main` branch and the latest tagged release are supported for security fixes.

## Reporting

Do not open public issues for undisclosed vulnerabilities. Report them privately to the maintainers with:

- affected version or commit
- reproduction steps
- impact assessment
- proposed mitigation if known

## Operational Security Baseline

- Remote deployments must use `LOCKFREE_USERS_FILE` and `LOCKFREE_SESSION_SECRET`.
- `LOCKFREE_ADMIN_API_TOKEN` is intended for automation and metrics scraping, not interactive browser access.
- Terminate TLS at the edge and set `LOCKFREE_TRUST_PROXY_HEADERS=true` only behind a trusted proxy.
- Rotate `LOCKFREE_SESSION_SECRET`, `LOCKFREE_ADMIN_API_TOKEN`, and user password hashes on a regular schedule.
