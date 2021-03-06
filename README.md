# 🤧 Hostan app

`/ˈhʊsˌtan/` app

![CI Status](https://github.com/lohmander/hostanapp/workflows/CI/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/lohmander/hostanapp)

Hostan provides a set of Kubernetes primitives to easily and rapidly deploy apps on a cluster that uses services like databases, object storage, etc. "Hostan" is a Swedish
word that translates to "the cough."

### Todo

- [x] Create README outlining the future functionality
- [x] Implement CRD and controller for the App resource
- [x] Implement CRD and controller for the Provider resource
- [x] Create spec/protobuf file for the provider interface
- [x] Create config maps and secrets for config upon reconciliation
- [x] Add support for cronjobs
- [ ] Add support for explicit environment variable definitions
- [ ] Add validation hooks for providers
- [ ] Implement proper statuses
- [x] Write a provider for SQL databases (using database/sql)
- [ ] Write a provider for redis

### What it looks like (WIP)

```yaml
apiVersion: hostan.hostan.app/v1
kind: App
metadata:
  name: my-app
spec:
  services:
    - name: webapp
      image: lohmander/webapp-image:latest
      port: 80
      env:
        - fromConfigMap: my-cm
        - fromSecret: my-secret
        - name: SOME_VAR
          value: some value
      ingress:
        host: myapp.hostan.app
  cronjobs:
    - name: backup
      image: lohmander/webapp-image:latest
      command: ["python", "manage.py", "dbbackup"]
      schedule: "0 0 * * *"
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

## 4. Providers

TBD

### Use existing

- PostgreSQL
- Redis
- S3

### Build your own provider

TBD
