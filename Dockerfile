FROM golang:1.27.1 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/ ./cmd/server ./cmd/ingestor

# Owned by nonroot so a fresh named volume mounted here is writable by the ingestor.
RUN mkdir -p /out/data/assets

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/server /server
COPY --from=build /out/ingestor /ingestor
COPY --from=build --chown=65532:65532 /out/data /data

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/server"]
