FROM golang:1.22.5-alpine

WORKDIR /journey

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

WORKDIR /journey/cmd/journey/main

RUN go build -o /journey/cmd/bin/main .

EXPOSE 8080

ENTRYPOINT [ "/journey/cmd/bin/main" ]