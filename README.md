# ACME webhook for Hetzner Cloud DNS API

This solver can be used when you want to use cert-manager with the [Hetzner Cloud DNS API](https://docs.hetzner.cloud/reference/cloud#dns).

Fork of [vadimkim/cert-manager-webhook-hetzner](https://github.com/vadimkim/cert-manager-webhook-hetzner), rewritten for the Hetzner Cloud API (`api.hetzner.cloud`). Hetzner migrated DNS management from `dns.hetzner.com` to `api.hetzner.cloud` in November 2025, and the upstream webhook does not support the new API.

## Requirements

- [go](https://golang.org/) >= 1.24.0
- [helm](https://helm.sh/) >= v3.0.0
- [kubernetes](https://kubernetes.io/) >= v1.22.0
- [cert-manager](https://cert-manager.io/) >= 0.12.0

## Installation

### cert-manager

Follow the [instructions](https://cert-manager.io/docs/installation/) using the cert-manager documentation to install it within your cluster.

### Webhook

#### Using public helm chart

```bash
helm repo add cert-manager-webhook-hetzner https://sshine.github.io/cert-manager-webhook-hetzner
helm install --namespace cert-manager cert-manager-webhook-hetzner cert-manager-webhook-hetzner/cert-manager-webhook-hetzner
```

#### From local checkout

```bash
helm install --namespace cert-manager cert-manager-webhook-hetzner deploy/cert-manager-webhook-hetzner
```

**Note**: The kubernetes resources used to install the Webhook should be deployed within the same namespace as the cert-manager.

To uninstall the webhook run

```bash
helm uninstall --namespace cert-manager cert-manager-webhook-hetzner
```

## Issuer

Create a `ClusterIssuer` or `Issuer` resource as following:
(Keep in mind that the example uses the Staging URL from Let's Encrypt. See [Getting Started](https://letsencrypt.org/getting-started/) for the production URL.)

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: mail@example.com
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourdomain.tld
            solverName: hetzner
            config:
              secretName: hetzner-secret
              # zoneName: example.com  # optional: auto-detected if omitted
              # apiUrl: https://api.hetzner.cloud/v1  # optional: defaults to Hetzner Cloud API
              # secretKey: api-token  # optional: defaults to "api-token"
```

### Credentials

In order to access the Hetzner Cloud API, the webhook needs an API token. Create a token at [Hetzner Cloud Console](https://console.hetzner.cloud/) with DNS write permissions.

If you choose another name for the secret than `hetzner-secret`, you must install the chart with a modified `secretName` value. RBAC policies ensure that no other secrets can be read by the webhook. Also modify the value of `secretName` in the `[Cluster]Issuer`.

The secret key in the secret defaults to `api-token` and can be overridden with the `secretKey` config field.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hetzner-secret
  namespace: cert-manager
type: Opaque
data:
  api-token: <your-hetzner-cloud-api-token-base64-encoded>
```

### Create a certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-cert
  namespace: cert-manager
spec:
  commonName: example.com
  dnsNames:
    - example.com
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
  secretName: example-cert
```

## Development

### Running the test suite

All DNS providers **must** run the DNS01 provider conformance testing suite, else they will have undetermined behaviour when used with cert-manager.

**It is essential that you configure and run the test suite when creating a DNS01 webhook.**

First, you need to have a Hetzner Cloud account with access to DNS. You need to create an API token and have a registered and verified DNS zone.
Then replace the `zoneName` parameter in `testdata/hetzner/config.json` with your actual zone. You also must encode your API token into base64 and put it in `testdata/hetzner/hetzner-secret.yml`.

You can then run the test suite with:

```bash
./scripts/fetch-test-binaries.sh
TEST_ZONE_NAME=example.com. make test
```

## Building

The CI workflow (`.github/workflows/build.yml`) builds multi-arch Docker images for `linux/amd64`, `linux/arm64`, and `linux/arm/v7` and pushes them to GHCR:

```
ghcr.io/sshine/cert-manager-webhook-hetzner
```

To build locally:

```bash
docker build -t cert-manager-webhook-hetzner .
```
