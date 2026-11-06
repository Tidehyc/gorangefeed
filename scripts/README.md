# Scripts Directory

This directory contains maintenance scripts for GoRangeFeed.

- `check_migrations.sh`: fails if any file under `migrations/` was modified or deleted relative to
  a target branch (default `origin/main`). Already-applied migrations must stay immutable — add a
  new numbered pair instead of editing an old one. Run it before opening a PR that touches
  `migrations/`.

Everything else is driven by the `Makefile` at the repository root — `make up` for the compose
stack, `make migrate` for schema migrations, `make test` for the unit/integration suites and
`make e2e` for the end-to-end suite. There is no separate deployment script: the system is meant
to run locally against `docker-compose.yml`.
