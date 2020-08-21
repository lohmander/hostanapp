# Host An App

Do you dig containers, but don't wanna learn about pods, services, and ingresses to deploy them? Yet alone realize that you need something called stateful sets to deploy your database, and God knows how to keep it backed up and secure. Sure something like AWS Lambda, or Google Cloud Run sounds good but you don't really want to be subject to provider lock-in. Maybe it'd be nice to be able to run on your machine, or take a peek into the source code when, if ever, interested?

Host An App aims to provide a trivially simple interface to deploy apps on Kubernetes, with all the bells and whistle using all open source software.

- [ ] PostgreSQL (ish, maybe CockroachDB)
- [ ] Redis
- [ ] S3 compatible object storage
- [ ] Let's Encrypt certificate provisioning
- [ ] Automatic backups of database

### How it looks

```yaml
name: awesome-app
units:
  - name: frontend
    image: docker.pkg.github.com/lohmander/awesome-app/frontend:v1
    port: 80
    domain: awesome.app
  - name: rest-api
    image: docker.pkg.github.com/lohmander/awesome-app/api:v1
    port: 8000
    domain: api.awesome.app
services:
  - postgresql
  - redis
  - s3
```
