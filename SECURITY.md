# Security policy

`rere` writes commits and pull requests into GitOps repositories using a
GitHub token, so we treat security reports seriously even pre-1.0.

## Supported versions

The project is in early development (pre-1.0); only the latest release and
`main` receive fixes.

## Reporting a vulnerability

Please **do not open a public issue** for vulnerabilities. Report privately
via [GitHub Security Advisories](https://github.com/Ca-moes/rere/security/advisories/new).
You should receive a response within a week.

## Token handling

`rere` reads its GitHub token only from an environment variable
(`git.auth.tokenEnv`); tokens must never be committed to config files, and
config validation rejects values that look like inline tokens.
