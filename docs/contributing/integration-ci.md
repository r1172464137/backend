# Testing changes to the fork's stable `main`

`main` and `master` are independent lines in this fork. For a push to `main`, CI compares the migration inventory with the **previous `main` commit** (`before`); for a pull request targeting `main`, it uses the PR base. The base must exist and be an ancestor. A missing migration, a migration in only one database dialect, or a new feature migration in the LTS number range fails the check. Other branches retain their comparison with `origin/master`.

Builds and checks use **Go 1.27.3**; the module minimum is `1.27.1`. The shared CI gate runs the tests with databases, the race detector, `go vet`, `make lint`, `make vulncheck`, protobuf regeneration and the fork's release-contract check. To regenerate bindings locally, use `make proto` with protoc 3.21.12; the Makefile supplies the matching Go generator and formatter.
