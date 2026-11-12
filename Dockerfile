# ---- build the Go API (with the embedded web frontend) ----
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rss-server ./cmd/server

# ---- minimal runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/rss-server /rss-server
EXPOSE 8080
ENTRYPOINT ["/rss-server"]
