export NAME=envoy-gateway
export NAMESPACE=cozy-$(NAME)

include ../../../hack/common-envs.mk
include ../../../hack/package.mk

# Pin the upstream gateway-helm chart by version AND OCI manifest digest.
#
# ENVOY_GATEWAY_CHART_DIGEST is the digest of the OCI **chart** manifest at
# docker.io/envoyproxy/gateway-helm (tag v1.9.3; upstream also publishes a
# "1.9.3" tag with a different digest, the documented one is "v1.9.3"). It is
# NOT the digest of a container image: the controller, ratelimit and proxy
# image digests live in values.yaml. All of them advance together at every
# bump. To bump:
#   crane digest docker.io/envoyproxy/gateway-helm:<new-tag>
#   crane digest docker.io/envoyproxy/gateway:<new-tag>
#   crane digest docker.io/envoyproxy/envoy:distroless-v<envoy version the
#     new release pins in api/v1alpha1/shared_types.go DefaultEnvoyProxyImage>
# and update both lines below plus the image pins in values.yaml.
ENVOY_GATEWAY_CHART_VERSION ?= v1.9.3
ENVOY_GATEWAY_CHART_DIGEST ?= sha256:be1b8f163a338f94e7d6532336c08d6856fc70a0b790ac14a2826a54d23e0dd3

test:
	helm unittest .

update:
	rm -rf charts
	mkdir -p charts
	# Pull by digest, then verify Chart.yaml records the expected version.
	helm pull oci://docker.io/envoyproxy/gateway-helm@$(ENVOY_GATEWAY_CHART_DIGEST) --untar --untardir charts
	# Pulling by digest leaves an empty directory named after the digest ref.
	rm -rf "charts/gateway-helm@$(ENVOY_GATEWAY_CHART_DIGEST)"
	@actual_version=$$(awk '/^version:/ {print $$2}' charts/gateway-helm/Chart.yaml | tr -d '"'); \
	if [ "$$actual_version" != "$(ENVOY_GATEWAY_CHART_VERSION)" ]; then \
	  echo "ERROR: pulled chart Chart.yaml version=$$actual_version but Makefile ENVOY_GATEWAY_CHART_VERSION=$(ENVOY_GATEWAY_CHART_VERSION). Bump both in lockstep."; \
	  exit 1; \
	fi
	# The Gateway API CRDs belong to cozystack.gateway-api-crds (same bundle
	# version today). A second release carrying them would, under the
	# CreateReplace CRD policy this package needs for its own CRDs, overwrite
	# the platform's copy on every upgrade. Only the gateway.envoyproxy.io
	# CRDs stay in the vendored crds subchart.
	rm -f charts/gateway-helm/charts/crds/crds/gatewayapi-crds.yaml
