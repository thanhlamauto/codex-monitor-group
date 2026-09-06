# Contributing

Contributions are welcome from everyone. You do not need to be a repository collaborator to propose a change: fork the public repository, create a branch, and open a pull request.

## Development workflow

1. Create a branch from the latest `main`.
2. Keep each pull request focused on one change.
3. Add or update tests for behavior changes.
4. Run the full test suite locally:

   ```bash
   python3 -m venv .venv
   . .venv/bin/activate
   pip install -r requirements.txt
   make test
   ```

5. Open a pull request and complete the checklist in the template.

GitHub Actions runs the server, Go agent, and installer checks for every pull request. Vercel creates a preview deployment when its Git integration is enabled. Changes reach production only after review, passing checks, and merge to `main`.

## Fair review policy

- The repository owner and collaborators follow the same protected-branch rules.
- Changes are merged through pull requests; direct pushes to `main` are disabled.
- At least one approving review is required.
- Stale approvals are dismissed when new commits materially change a pull request.
- Review conversations must be resolved before merge.
- Review the code, privacy impact, integrity behavior, and tests rather than the contributor's identity.

## Privacy and security

Never commit credentials, Vercel tokens, database URLs, device private keys, raw Codex logs, prompts, responses, source code from monitored machines, or local `.env` files. Use placeholders in `.env.example` and GitHub/Vercel secret stores for deployment credentials.

For a security-sensitive report that would put users at risk if disclosed immediately, contact the repository owner privately before publishing exploit details.
