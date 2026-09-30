FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy \
 && CGO_ENABLED=0 go build -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -o /out/submission ./cmd/submission \
 && CGO_ENABLED=0 go build -o /out/voting ./cmd/voting \
 && CGO_ENABLED=0 go build -o /out/orchestrator ./cmd/orchestrator \
 && CGO_ENABLED=0 go build -o /out/canvas ./cmd/canvas \
 && CGO_ENABLED=0 go build -o /out/archive ./cmd/archive \
 && CGO_ENABLED=0 go build -o /out/realtime ./cmd/realtime \
 && CGO_ENABLED=0 go build -o /out/load ./cmd/load

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/ /app/
COPY data /app/data
