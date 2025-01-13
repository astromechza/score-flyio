# score-flyio

‼️ In development.

This repo is forked from the <https://github.com/score-spec/score-implementation-sample> template. The intention is for this to be a valid Score implementation that can construct flyctl config files and use resource provisioning and all the platform specific features provided by Fly. However, since Fly is considerably different to Kubernetes or Docker Compose, certain Score features will not be available and will be rejected if used in workloads.

This is a rewrite of <https://github.com/astromechza/score-flyio-archived> since the Score spec has moved on and our understanding of resource provisioning and Score feature compatibility is more complete now.

### Workflow

Initialize the project directory. Because app names must be globally unique, you may need to use the `--fly-app-prefix` to add to the front of the Score workload names.

```
score-flyctl init --fly-app-prefix my-app-prefix-
```

Then generate the output Fly toml files per Score workload:

```
score-flyctl generate *.score.yaml
```

Then ensure the Fly app exists if it doesn't already and assign a shared ip if needed for each app:

```
fly apps create my-app-prefix-example-workload
fly ip allocate-v4 -a my-app-prefix-example-workload --shared
```

Now we can stage our secrets and deploy our workloads:

```
cat fly_example-workload.env | fly secrets import -a my-app-prefix-example-workload --stage 
fly deploy -c fly_example-workload.toml
```

See [./samples](./samples) for some sample Score apps that we use during testing to check the conversion process. These should all be deployable.

### Supported 🟢

- A single workload container
- Setting a container image or using a local Dockerfile+.dockerignore built by Fly.io on deploy
- Setting `command` and `args` overrides
- Setting `variables` for environment variables including placeholders
- Setting cpu and memory resources in rounded multiples of 1 cpu, 256MB memory using the maximum of resource requests and resource limits if defined
- Mounting files
- Mounting a named Fly.io volume
- Exposing tcp and udp network services with annotations for enabling Fly Proxy handlers
- Converting liveness and readiness http get probes into Fly checks
- Resource Provisioning using static json, command execution, or HTTP request
- Secret variables and mounted files when they contain secret outputs from resources

### Not supported 🔴

- Multiple workload containers (This may improve once https://community.fly.io/t/docker-without-docker-now-with-containers/22903 is released in Fly.io)
- Setting the mode for mounted files (not supported by Fly)
- Setting the subpath or enabling readonly on mounted volumes (not supported by Fly)
