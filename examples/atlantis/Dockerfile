# Atlantis-compatible image bundling c3x.
#
# Builds on top of the official Atlantis image so all the runner
# tooling (terraform, conftest, etc.) stays present, then layers
# the c3x binary on top.
#
# Pin a specific Atlantis tag in production deployments rather
# than `:latest`; this Dockerfile is a starting point.

FROM golang:1.23 AS c3x-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/c3x ./cmd/c3x

# Atlantis ships their image as ghcr.io/runatlantis/atlantis.
FROM ghcr.io/runatlantis/atlantis:latest

# /usr/local/bin is on the runner's PATH; copies are non-root-safe
# because the base image already runs as `atlantis` non-root user.
COPY --from=c3x-builder /out/c3x /usr/local/bin/c3x

# Smoke check at build time so a missing binary fails fast.
RUN c3x --version
