# Build stage
#
# Every dependency is public, so no registry credentials are needed.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /wagering ./cmd/wagering

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /wagering /usr/local/bin/wagering
ENTRYPOINT ["wagering"]
