-include .env.local

# get the current OS and arch
OS=$(shell GOOS= go env GOOS)
ARCH=$(shell GOARCH= go env GOARCH)

# get the target OS and arch
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

lint:
	golangci-lint run

static:
	CGO_ENABLED=0 go build -tags='netgo timetzdata' -trimpath -ldflags "-s -w" -o ./bin/$(GOOS)_$(GOARCH)/ ./cmd/scanner

package: static
	mkdir -p dist
	@if [ "${GOOS}" = "darwin" ] && [ "${OS}" != "darwin" ]; then \
		echo "darwin must be packaged on macOS"; \
	elif [ "${GOOS}" = "darwin" ] && [ "${OS}" = "darwin" ]; then \
		codesign --deep -s $(APPLE_CERT_ID) -o runtime bin/$(GOOS)_$(GOARCH)/scanner; \
		ditto -ck bin/$(GOOS)_$(GOARCH) dist/sia-wallet-scanner_$(GOOS)_$(GOARCH).zip; \
		xcrun altool --notarize-app --primary-bundle-id com.siacentral.sia-wallet-scanner --apiKey $(APPLE_API_KEY) --apiIssuer $(APPLE_API_ISSUER) --file dist/sia-wallet-scanner_darwin_amd64.zip; \
		rm -rf bin/$(GOOS)_$(GOARCH); \
	else \
		zip -qj dist/sia-wallet-scanner_$(GOOS)_$(GOARCH).zip bin/$(GOOS)_$(GOARCH)/*; \
		rm -rf bin/$(GOOS)_$(GOARCH); \
	fi

# must be run on macOS
apple-notarize-status:
	@if [ "${OS}" = "darwin" ]; then \
		xcrun altool --notarization-info $(APPLE_NOTARIZE_UUID) --apiKey $(APPLE_API_KEY) --apiIssuer $(APPLE_API_ISSUER); \
	else \
		echo "Notarization is only supported on macOS"; \
	fi

release:
	rm -rf bin dist
	@for OS in linux darwin windows; do \
		for ARCH in amd64 arm64; do \
			GOOS=$$OS GOARCH=$$ARCH make package; \
		done; \
	done
	rm -rf bin

docker:
	docker build -t siacentral/host-manager:$(GIT_REVISION) .