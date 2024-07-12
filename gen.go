package main

// go:generate migrate --migrations ./internal/pgstore/migrations --config ./internal/pgstore/migrations/tern.conf

// go:generate sqlc generate -f ./internal/pgstore/sqlc.yaml
