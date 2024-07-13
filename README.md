# nlw-journey-go

package(s):
    [goapi-gen](https://github.com/discord-gophers/goapi-gen)
    Geracão de body-plate
    Comandos:
        - go install github.com/discord-gophers/goapi-gen@latest
        - goapi-gen --out ./internal/api/spec/journey.gen.spec.go ./internal/api/spec/openapi_journey.spec.json

        go mod tidy
        go get -u ./...

    [sqlc](https://github.com/sqlc-dev/sqlc)
    Geracão de código tipado à partir de código SQL
    Comandos:
        go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
        go get -u ./...
        sqlc generate -f ./internal/pgstore/sqlc.yaml
        
    [tern](https://github.com/jackc/tern)
    Geracão de migrations
    Comandos:        
        - go install github.com/jackc/tern/v2@latest
        - tern init {output directory}
          ```shell tern init ./internal/pgstore/migrations```

          - criado variaveis de ambiente na maquina de desenvolvimento
          ```shell
                # Environment variables to journey app - nlw rocketseat
                export JOURNEY_DATABASE_HOST="localhost"
                export JOURNEY_DATABASE_PORT=5432
                export JOURNEY_DATABASE_NAME="journey"
                export JOURNEY_DATABASE_USER="postgres"
                export JOURNEY_DATABASE_PASSWORD="123456789"
          ```
          - run/up database service using docker-compose
          - criar as migrations usando tern
            ```bash tern new --migrations {output} {migration's name} ```
            Ex.: ```bash tern new --migrations ./initial/pgstore/migrations create_trips_table```
            
            go generate ./..

