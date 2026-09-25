SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

IMAGE_REPOSITORY ?= YOUR_IMAGE_NAME_AND_REGISTRY_URL
IMAGE_TAG ?= $(IMAGE_REPOSITORY):v1.0.0-alpha2-demo1
MODEL_NAME ?= gpt-4.1-mini

.PHONY: setup test build push render deploy clean
setup:
	go mod download

test:
	go test ./...

build:
	docker buildx build --platform linux/amd64 --load -t "$(IMAGE_TAG)" .

push:
	docker push "$(IMAGE_TAG)"

render:
	@test -n "$(IMAGE)" || { printf 'set IMAGE to the repository@sha256 digest of the pushed image\n' >&2; exit 1; }
	@test -n "$(SNAPSHOT_LOCATION)" || { printf 'set SNAPSHOT_LOCATION to a writable Substrate snapshot URI\n' >&2; exit 1; }
	@IMAGE="$(IMAGE)" SNAPSHOT_LOCATION="$(SNAPSHOT_LOCATION)" MODEL_NAME="$(MODEL_NAME)" \
		envsubst '$${IMAGE} $${SNAPSHOT_LOCATION} $${MODEL_NAME}' < agent.yaml.tmpl

deploy:
	kubectl apply --dry-run=server -f agent.yaml
	kubectl apply -f agent.yaml

clean:
	kubectl delete -f agent.yaml
