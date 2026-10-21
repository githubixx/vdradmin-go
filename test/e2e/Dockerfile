ARG FIXTURE_BUILDER=fixture-builder
FROM golang:1.26.0-bookworm AS fixture-source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM fixture-source AS fixture-builder
RUN sh scripts/check-release-readiness.sh && CGO_ENABLED=0 go build -mod=readonly -o /ui-fixture ./test/e2e/server

FROM fixture-source AS fixture-builder-workspace
COPY --from=chroma-ui go.mod go.sum /chroma-ui/
COPY --from=chroma-ui *.go /chroma-ui/
COPY --from=chroma-ui assets /chroma-ui/assets
COPY --from=chroma-ui templates /chroma-ui/templates
RUN go work init /src /chroma-ui && CGO_ENABLED=0 go build -mod=readonly -o /ui-fixture ./test/e2e/server

FROM ${FIXTURE_BUILDER} AS fixture-selected

FROM mcr.microsoft.com/playwright:v1.64.1-noble@sha256:7852dc5a7d72f5fe458a597d690f6c42fbc8e9e26ddfa95f8e6ef552558d9bbf
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY test/e2e/resources.mjs test/e2e/resources.test.mjs test/e2e/resources.lock.json ./test/e2e/
COPY test/e2e/resources ./test/e2e/resources
RUN npm run test:e2e:resources:test && npm run test:e2e:resources
COPY --from=fixture-selected /ui-fixture /usr/local/bin/ui-fixture
COPY go.mod ./
COPY web ./web
COPY playwright.config.js ./
COPY test/e2e/specs ./test/e2e/specs
RUN mkdir -p test/e2e/server && chmod 1777 test/e2e/server
ENV E2E_SERVER_COMMAND="/usr/local/bin/ui-fixture --addr 127.0.0.1:18763"
ENV TZ=UTC
ENV E2E_SCREENSHOT_RUNTIME=playwright-1.64.1-noble
CMD ["npm", "run", "test:e2e"]
