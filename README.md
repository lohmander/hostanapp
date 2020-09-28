# Host An App

Provide a set of Kubernetes primitives to easily and rapidly deploy apps on a cluster.

### What it looks like (WIP)

```yaml
apiVersion: hostan.app/v1alpha1
kind: App
metadata:
  name: my-app
spec:
  services:
    - name: frontend
      image: lohmander/frontend-image:latest
      ingress:
        host: myapp.hostan.app
    - name: api
      image: lohmander/api-image:latest
      ingress:
        host: myapp.hostan.app
        path: /api/v1
  tasks:
    - name: send-messages
      schedule: * * 1 * * *
      image: lohmander/api-image:latest
      command: ["/main", "sendmessages"]
  uses:
    - name: postgresql
    - name: redis
      config:
        memory: 128mb
```
