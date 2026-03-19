FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o bbqbookkeeper .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bbqbookkeeper .
COPY ui/ ./ui/
EXPOSE 8080
ENTRYPOINT ["./bbqbookkeeper"]