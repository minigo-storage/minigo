.PHONY: help build images smoke preview security image-security compatibility reference-binaries reference-images reference-smoke

help:
	@echo 'make build               Build the console and Minigo Linux amd64/arm64 binaries'
	@echo 'make images              Build and load local Minigo images'
	@echo 'make smoke               Execute both Minigo architectures with real mc operations'
	@echo 'make compatibility       Run existing-volume image-swap acceptance tests'
	@echo 'make preview             Start a persistent LAN-accessible local console'
	@echo 'make security            Check the shipped Go code and frontend dependencies'
	@echo 'make image-security      Scan both local images and retain full raw findings'
	@echo 'make reference-binaries  Build the MinIO reference for Linux amd64 and arm64'
	@echo 'make reference-images    Build and load both reference images locally'
	@echo 'make reference-smoke     Test startup, health and S3 operations on both images'

build:
	./scripts/build.sh

images: build
	./scripts/build-images.sh

smoke:
	MINIGO_IMAGE_PREFIX=minigo ./scripts/smoke-reference.sh

compatibility:
	./scripts/test-compatibility.sh

preview:
	./scripts/preview.sh

security:
	./scripts/check-security.sh

image-security:
	./scripts/check-image-security.sh

reference-binaries:
	./scripts/build-reference.sh

reference-images: reference-binaries
	./scripts/build-reference-images.sh

reference-smoke:
	./scripts/smoke-reference.sh
