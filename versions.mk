DRIVER_NAME := fake-dra-driver
MODULE := github.com/toVersus/$(DRIVER_NAME)

VERSION  ?= v0.1.0
# vVERSION represents the version with a guaranteed v-prefix
vVERSION := v$(VERSION:v%=%)

GOLANG_VERSION ?= 1.22.1

# These variables are only needed when building a local image
CLIENT_GEN_VERSION ?= v0.29.2
CONTROLLER_GEN_VERSION ?= v0.14.0
GOLANGCI_LINT_VERSION ?= v1.52.0
MOQ_VERSION ?= v0.3.4

BUILDIMAGE ?= ghcr.io/toVersus/fake-dra-driver:devel-go$(GOLANG_VERSION)

GIT_COMMIT ?= $(shell git describe --match="" --dirty --long --always --abbrev=40 2> /dev/null || echo "")
