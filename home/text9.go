I’m ready to build the API, but I need the BRD and repo details to proceed.

Please provide:
1) BRD (paste under these exact headings)
- Overview
- Problem Statement
- Objective
- Functional Requirements
- Data Requirements
- Business Rules
- API Requirements
- Acceptance Criteria

2) SCM / GitHub details
- Repo URL:
- Branch:
- Git auth (PAT/SSH): 
- Commit author name/email (optional):
- Any required Git org protections (e.g., PR only, signed commits):

3) Tech choices (or I’ll use sane defaults)
- Database (Postgres | MySQL | Redshift). If Postgres, default.
- Auth (JWT | API key | none). If unspecified, I’ll implement JWT-ready middleware with optional bypass.
- Go version (default: 1.22), module name (e.g., github.com/yourorg/yourservice).
- Logging (zerolog default) and config via env with Viper.
- Migrations (golang-migrate) and driver (pgx for Postgres).
- Any rate limiting or audit logging needs.

If you prefer, I can first scaffold a clean, production-ready Go/Mux API template (config, logging, error handling, repo/service layout, tests, migrations, Makefile, Dockerfile, CI stub) and then implement your domain once the BRD arrives.