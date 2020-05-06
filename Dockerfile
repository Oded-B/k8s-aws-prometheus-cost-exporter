FROM golang:1.14.2-alpine AS build_base

RUN apk add git

WORKDIR /go/src/github.com/Oded-B/k8s-aws-promtheus-cost-exporter
ENV GO111MODULE=on

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_base AS code_builder
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o /go/bin/k8s-aws-promtheus-cost-exporter

FROM alpine AS k8s-aws-promtheus-cost-exporter
COPY --from=code_builder /go/bin/k8s-aws-promtheus-cost-exporter /go/bin/k8s-aws-promtheus-cost-exporter
ENTRYPOINT ["/go/bin/k8s-aws-promtheus-cost-exporter"]
