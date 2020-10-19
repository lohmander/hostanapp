# Hostan app

`/ˈhʊsˌtan/` app

Hostan provides a set of Kubernetes primitives to easily and rapidly deploy apps on a cluster that uses services like databases, object storage, etc. "Hostan" is a Swedish
word that translates to "the cough."

### Todo

- [x] Create README outlining the future functionality
- [x] Implement CDR and controller for the App resource
- [ ] Implement CDR and controller for the Provider resource
- [ ] Create spec/protobuf file for the provider interface
- [ ] Create config maps and secrets for config upon reconciliation

### What it looks like (WIP)

```yaml
apiVersion: hostan.app/v1alpha1
kind: App
metadata:
  name: my-app
spec:
  services:
    - name: webapp
      image: lohmander/webapp-image:latest
      port: 80
      ingress:
        host: myapp.hostan.app
  uses:
    - name: postgresql
    - name: redis
    - name: s3
      config:
        endpointUrl: https://fra1.digitaloceanspaces.com
```

## Table of contents

1. Installation
2. Getting started
3. Resource types
   - App
   - Provider
   - Snapshot
4. Providers
   - Use existing
   - Build your own provider

## 1. Installation

TBD

## 2. Getting started

TBD

## 3. Resource types

TBD

### App

TBD

### Provider

TBD

### Snapshot

TBD

## 4. Providers

TBD

### Use existing

- PostgreSQL
- Redis
- S3

### Build your own provider

TBD
