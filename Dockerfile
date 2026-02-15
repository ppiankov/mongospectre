FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /mongospectre ./cmd/mongospectre/

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /mongospectre /usr/local/bin/mongospectre

USER nonroot:nonroot

ENTRYPOINT ["mongospectre"]
