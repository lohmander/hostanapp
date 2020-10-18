# Hostan app 
`/ˈhʊsˌtan/` app

Hostan provides a set of Kubernetes primitives to easily and rapidly deploy apps on a cluster. "Hostan" is a Swedish
word that translates to "the cough."

### Todo

- [x] Create README outlining the future functionality
- [ ] Implement CDR and controller for the App resource
- [ ] Implement CDR and controller for the Provider resource
- [ ] Create spec/protobuf file for the provider interface

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
```
