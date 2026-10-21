---
applyTo: "cmd/**"
---

# Composition root

- `cmd/gobuzzkey` wires adapters to use cases, parses flags, prints results and maps errors to exit statuses. Business rules belong in `app` or `domain`.
- The environment is read only through `env.getenv`, populated from `os.Getenv` in `main.go`. Output goes to `env.stdout` / `env.stderr`.
- No flag accepts a secret key. The owner key comes from the prompt, a pipe or `BUZZ_OWNER_PRIVATE_KEY`.
- When the identity goes to stdout (`-out -`), every other line goes to stderr so the stdout stream stays parseable.
- Every relay-supplied string printed (channel names and IDs, event IDs, rejection messages) and every error line goes through `shared.Printable`.
- Ctrl-C exits 130; the prompt restores the terminal echo state before returning.
- Flags, defaults, exit statuses and hints match the README. A new subcommand or flag gets a README entry and a `main_test.go` case.
