.PHONY: help reference-binaries reference-images reference-smoke

help:
	@echo 'make reference-binaries  Build the MinIO reference for Linux amd64 and arm64'
	@echo 'make reference-images    Build and load both reference images locally'
	@echo 'make reference-smoke     Test startup, health and S3 operations on both images'

reference-binaries:
	./scripts/build-reference.sh

reference-images: reference-binaries
	./scripts/build-reference-images.sh

reference-smoke:
	./scripts/smoke-reference.sh
