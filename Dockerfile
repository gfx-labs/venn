# syntax=docker/dockerfile:1
FROM golang:1.23-alpine as GOBUILDER
RUN apk add build-base git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY svc svc
COPY lib lib
COPY cmd cmd
COPY dashboard dashboard

RUN go build -o venn ./cmd/venn

FROM alpine:latest
WORKDIR /
RUN apk add --no-cache bash dumb-init

COPY --from=GOBUILDER /src/venn /usr/bin/venn

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/bin/venn"]

