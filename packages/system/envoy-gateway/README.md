# envoy-gateway

Envoy Gateway v1.9.3 (vendored `gateway-helm` chart) in Gateway Namespace Mode,
plus the GatewayClass `envoy-internal` and its `EnvoyProxy` (distroless Envoy,
LoadBalancer Service with `externalTrafficPolicy: Local` and no NodePorts).

It backs INTERNAL tenant Gateways. Every Gateway of the bundled `cilium` class
shares one per-node Envoy, so an organisation's Gateway there cannot be isolated
from another's. With this class each Gateway gets its own proxy Deployment and
LoadBalancer Service in the tenant namespace, which the per-tenant gateway chart
fences with the CiliumNetworkPolicy `cozystack-gateway-isolation`.

## Enabling

In the platform values (`spec.components.platform.values` of the
`cozystack.cozystack-platform` Package):

```yaml
gateway:
  internal:
    enabled: true                     # emits the cozystack.envoy-gateway Package
  tenantSelectableClasses: [envoy-internal]   # lets a tenant pick it (tenant.spec.gatewayClass)
  isolatedClasses: [envoy-internal]           # declares it internal (required alongside)
publishing:
  certificates:
    solver: dns01                     # HTTP-01 never validates a private address
```

An isolated class must also be tenant-selectable, `envoy-internal` must be in
both lists or neither, it may never be `gateway.className`, and the publishing
tenant may not use it; all are refused at render time. The isolation policy
itself is rendered for every tenant Gateway (it selects nothing on `cilium`).

The controller namespace carries its own CiliumNetworkPolicy
(`envoy-gateway-controller`): only Envoy proxy pods reach the xDS port
(18000/TCP) and only the kubelet reaches the probe port (8081/TCP). The
ratelimit, Wasm and metrics ports are closed, so a rate-limit backend, Wasm
extensions or scraping the controller's metrics need that policy extended.

## What a tenant Gateway on this class gets

In the Gateway's namespace, all named after the Gateway (`cozystack`): a
Deployment, a Service of type LoadBalancer, a ServiceAccount and a ConfigMap
(SDS config and the xDS CA). The Service and the pods carry:

```
app.kubernetes.io/name: envoy
app.kubernetes.io/component: proxy
app.kubernetes.io/managed-by: envoy-gateway
gateway.networking.k8s.io/gateway-name: cozystack
gateway.envoyproxy.io/owning-gateway-name: cozystack
gateway.envoyproxy.io/owning-gateway-namespace: <tenant namespace>
```

The Service has no MetalLB annotation: the address pool is chosen by a MetalLB
pool `serviceSelector` on the two `owning-gateway-*` labels.

The proxy reaches the controller at
`envoy-gateway.cozy-envoy-gateway.svc.<cluster domain>:18000` (xDS over TLS,
authenticated with a projected ServiceAccount token).

## Updating

`make update` pulls the chart by digest and drops its Gateway API CRDs, which
`cozystack.gateway-api-crds` owns. Image pins (tag + index digest) live in
`values.yaml`; see the Makefile for the bump procedure.
