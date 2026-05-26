# Releasing

## Versioning

This repository uses semantic versioning for tagged releases.

## Pre-release Checklist

1. Run `go test ./...`
2. Run `go test -race ./...`
3. Run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
4. Run `cd web && npm audit --json`
5. Run `cd web && npm run smoke`
6. Run `go test ./web -run TestAuthenticatedServiceSoak`

## Artifact Expectations

- container image built from `Dockerfile`
- release notes describing API, security, and operational changes
- updated docs if configuration or deployment contracts changed

## Breaking Changes

Any breaking change to public library behavior, web authentication, persistence schema, or deployment configuration must be documented in the release notes and migration guidance.
