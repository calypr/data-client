FROM quay.io/cdis/golang:1.23-bookworm as build-deps

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR $GOPATH/src/github.com/calypr/calypr-cli/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN COMMIT=$(git rev-parse HEAD); \
    VERSION=$(git describe --always --tags); \
    printf '%s\n' 'package cmd'\
    ''\
    'const ('\
    '    gitcommit="'"${COMMIT}"'"'\
    '    gitdate="'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"'\
    '    gitversion="'"${VERSION}"'"'\
    ')' > cmd/gitversion.go \
    && go build -o /calypr-cli .

FROM scratch
COPY --from=build-deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build-deps /calypr-cli /calypr-cli
CMD ["/calypr-cli"]
