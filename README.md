# nlw-journey-go

package(s):
[goapi-gen](https://github.com/discord-gophers/goapi-gen)

Geracão de body-plate

Comandos:

```shell
go install github.com/discord-gophers/goapi-gen@latest
```
```shell
goapi-gen --out ./internal/api/spec/journey.gen.spec.go ./internal/api/spec/openapi_journey.spec.json

go mod tidy

go get -u ./...
```

[sqlc](https://github.com/sqlc-dev/sqlc)
Geracão de código tipado à partir de código SQL
Comandos:

```shell
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go get -u ./...
sqlc generate -f ./internal/pgstore/sqlc.yaml
```
    
[tern](https://github.com/jackc/tern)
Geracão de migrations
Comandos:
```shell
go install github.com/jackc/tern/v2@latest
tern init {output directory}
tern init ./internal/pgstore/migrations
tern migrate --migrations ./internal/pgstore/migrations/ --config ./internal/pgstore/migrations/tern.conf
```

- criado variaveis de ambiente na maquina de desenvolvimento
```shell
# Environment variables to journey app - nlw rocketseat
export JOURNEY_DATABASE_HOST="localhost"
export JOURNEY_DATABASE_PORT=5432
export JOURNEY_DATABASE_NAME="journey"
export JOURNEY_DATABASE_USER="postgres"
export JOURNEY_DATABASE_PASSWORD="123456789"
```
echo $JOURNEY_DATABASE_USER, $JOURNEY_DATABASE_PASSWORD, $JOURNEY_DATABASE_HOST, $JOURNEY_DATABASE_PORT, $JOURNEY_DATABASE_NAME

env -u JOURNEY_DATABASE_USER \
env -u JOURNEY_DATABASE_PASSWORD \
env -u JOURNEY_DATABASE_HOST \
env -u JOURNEY_DATABASE_PORT \ 
env -u JOURNEY_DATABASE_NAME


- run/up database service using docker-compose
- criar as migrations usando tern
```shell 
tern new --migrations {output} {migration's name} 
```
```shell 
bash tern new --migrations ./initial/pgstore/migrations create_trips_table
go generate ./..
```
