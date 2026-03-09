# Stage 1: Build
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/server

# Stage 2: Runtime
FROM gcr.io/distroless/static
COPY --from=build /app /app
EXPOSE 8080
CMD ["/app"]
