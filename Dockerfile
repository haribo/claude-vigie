# Multi-stage build for the claude-fleetd server, shipped as a snapshot image
# (a release artifact, not a hosted service). Static binary (CGO disabled) on a
# distroless nonroot base. The client (claude-fleet) is not containerized — it
# runs on each machine alongside Claude Code.
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Injected by the snapshot workflow (develop-<date>-<sha7>); "dev" for a plain
# local `docker build`.
ARG STAMP=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X github.com/haribo/claude-fleet/internal/version.Version=${STAMP}" \
      -o /out/claude-fleetd ./cmd/claude-fleetd
# Empty data dir owned by the nonroot uid, so the SQLite file is writable at runtime.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/claude-fleetd /usr/local/bin/claude-fleetd
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
# The auth token is supplied via FLEET_TOKEN (else one is generated and logged).
# Override the command to change --addr / --db, and mount a volume at /data to
# persist the database.
ENTRYPOINT ["/usr/local/bin/claude-fleetd"]
CMD ["serve", "--addr", ":8080", "--db", "/data/claude-fleet.db"]
