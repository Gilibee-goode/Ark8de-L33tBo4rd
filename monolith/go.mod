// go.mod — Go module definition for the Ark8de monolith
//
// A Go module is the unit of versioning and dependency management in Go.
// This file declares:
//   - The module path (used as the import prefix for all packages in this module)
//   - The minimum Go version required
//   - All external dependencies and their exact versions
//
// Run `go mod tidy` to:
//   - Download any missing dependencies
//   - Remove dependencies that are no longer used
//   - Generate/update go.sum (the lockfile with cryptographic hashes)

module github.com/Gilibee-goode/ark8de-l33tbo4rd

go 1.24.0

require (
	github.com/go-chi/chi/v5 v5.2.5
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/jackc/pgx/v5 v5.7.4
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lib/pq v1.10.9 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)
