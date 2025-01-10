# score-flyio

‼️ In development.

This repo is forked from the <https://github.com/score-spec/score-implementation-sample> template. The intention is for this to be a valid Score implementation that can construct flyctl config files and use resource provisioning and all the platform specific features provided by Fly. However, since Fly is considerably different to Kubernetes or Docker Compose, certain Score features will not be available and will be rejected if used in workloads.

This is a rewrite of <https://github.com/astromechza/score-flyio-archived> since the Score spec has moved on and our understanding of resource provisioning and Score feature compatibility is more complete now.

### Supported 🟢

Nothing.

### Not supported 🔴

Everything.

## Notes

The general output artefact should be a flyctl config file. During provisioning, secrets might be set in the app. To start with, provisioners will simply output plaintext or secret values.

Cannot support more than one container until https://community.fly.io/t/docker-without-docker-now-with-containers/22903 is released for flyctl. Also see https://community.fly.io/t/everybody-gets-containers-sidecars-and-init-containers-in-fks/23020.

Track https://fly-changelog.fly.dev/ for new changes to Fly platform and flyctl.

Support files via https://fly.io/docs/reference/configuration/#the-files-section.

For volumes, assume the source already exists as a fly volume of the same name. Allow provisioners to generate then name if necessary.

Use https://fly.io/docs/flyctl/secrets/ to set secret environment variables in the app. This may require us to hold secret values on the file system as they have been pulled from provisioners and then have a separate CLI command to converge the secrets prior to deployment.

```
score-flyctl init
score-flyctl generate *.score.yaml
score-flyctl export-secrets
flyctl volume create fizzy
flyctl deploy -c app-one.toml
flyctl deploy -c app-two.toml
score-flyctl remove-unused-secrets
```

Start with simple template provisioners that support values and secrets.
