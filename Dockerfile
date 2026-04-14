FROM golang:1.24-alpine AS builder
ARG BUILD_VERSION=good
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
# Build with tags: "bad" for bad performance version, default (no tag) for good version
RUN if [ "$BUILD_VERSION" = "bad" ]; then \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=bad -o bbqbookkeeper . ; \
    else \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bbqbookkeeper . ; \
    fi

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bbqbookkeeper .
COPY ui/ ./ui/
EXPOSE 8080
ENTRYPOINT ["./bbqbookkeeper"]