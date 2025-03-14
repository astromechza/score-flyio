# score-flyio

This repo is forked from the <https://github.com/score-spec/score-implementation-sample> template. The intention is for this to be a valid Score implementation that can construct flyctl config files and use resource provisioning and all the platform specific features provided by Fly. However, since Fly is considerably different to Kubernetes or Docker Compose, certain Score features will not be available and will be rejected if used in workloads.

This is a rewrite of <https://github.com/astromechza/score-flyio-archived> since the Score spec has moved on and our understanding of resource provisioning and Score feature compatibility is more complete now.

## Installation

Download and extract the binary from the latest release on GitHub: https://github.com/astromechza/score-flyio/releases. Or build from source via `go install github.com/astromechza/score-flyio@latest`.

### Workflow

Initialize the project directory. Because app names must be globally unique in Fly, you may need to use the `--fly-app-prefix` to add to the front of the Score workload names. This prefix should also be used by provisioners for namespacing any other apps created for this project.

```
score-flyio init --fly-app-prefix my-app-prefix-
```

Then generate the output Fly toml files per Score workload, set the secrets on the app, and deploy the app all in one command:

```
export FLY_API_TOKEN=$(fly tokens create org -x '24h' -o personal)
export FLY_REGION_NAME=lhr
score-flyio generate score.yaml --deploy
```

Then assign a shared ip if needed for the app that needs ingress networking:

```
fly ip allocate-v4 -a my-app-prefix-example-workload --shared
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

## Supported Workload annotations

`score-flyio` supports the following workload annotations that will modify the runtime behavior of the application when the annotations are found in the Workload metadata:

**`score-flyio.astromechza.github.com/service-<portname>-handlers`**

Expects a comma-seperated list of [Fly Proxy connection handlers](https://fly.io/docs/reference/fly-proxy/#connection-handlers) and will add these to the `[[service.ports]]` entry for the port.

For example, `score-flyio.astromechza.github.com/service-web-handlers: tls,http`.

**`score-flyio.astromechza.github.com/service-<portname>-http-options`**

Expects a serialized JSON payload that can contain the `http_service.http_options` attributes as documented in [the docs](https://fly.io/docs/reference/configuration/#http_service-http_options-idle_timeout).

For example, `score-flyio.astromechza.github.com/service-web-http-options: '{"idle_timeout": 60}'`.

**`score-flyio.astromechza.github.com/service-<portname>-auto-stop`**

Enables Fly Proxy based auto-stop to the string set in this attribute. This also enables auto-start.

For example, `score-flyio.astromechza.github.com/service-web-auto-stop: stop`.

**`score-flyio.astromechza.github.com/service-<portname>-min-running`**

Sets the minimum number of machines that must remain running.

For example, `score-flyio.astromechza.github.com/service-web-min-running: "1"`.

**`score-flyio.astromechza.github.com/service-<portname>-concurrency`**

Sets the Fly Proxy request routing concurrency for load balancing requests between machines. This expects a JSON payload.

For example, `score-flyio.astromechza.github.com/service-web-concurrency: '{"type": "requests", "hard_limit": 25, "soft_limit": 20}'`.

## State storage

All state including secret values from resource provisioners, are stored in the local `.score-flyio/state.yaml` file. When deploying as part of a CI pipeline, this file is vital to keep safe and control access to. This is similar to a Terraform or OpenTofu state file stored locally. This file should be maintained per deployment environment. Since it may contain unique ids and random data that cannot be retrieved once lost.

Some recommended methods of storing this file:

- Object Storage
- local persistent volume
- password manager such as 1password
- Hashicorp Vault
- Consul

In the future we may build state file download, upload, and mediation into `score-flyio`.

## Resource Provisioning

**NOTE**: this is described in more detail in the Score documentation: <https://docs.score.dev/docs/>.

You can request the provisioning of a resource by adding it to the `resources` section of your Score file:

```yaml
# ...
resources:
  db:
    type: postgres
    # class: <an optional subtype of postgres>
    # id: <an id that can be used between workloads to refer to the same instance of postgres database>
    # params: {} # postgres has no params
```

When you run `score-flyio generate`, the CLI will attempt to provision each resource using one of its configured provisioners.

Your app can then consume outputs from the resource in either the container variables or mounted container files sections:

```yaml
containers:
  main:
    # ...
    variables:
      DB_URL: postgres://${resources.db.user}:${resources.db.password}@${resources.db.host}:${resources.db.port}/${resources.db.name}
```

This sets the `[env]` section in the output `.toml` Fly.io configuration. If the resource marks one of the outputs as "secret", the CLI writes the secret in `KEY=VALUE` form to the `.env` file that accompanies your workload so that you can set it in Fly.io using `fly secrets import`.

You can configure 3 kinds of provisioners in `score-flyio`:

- `cmd` - will execute a binary with fixed args
- `http` - will issue HTTP POST requests to a target URL
- `static` - sets a static JSON map as the resource outputs

### Configuring provisioners

The CLI does not configure any provisioners by default. You can configure provisioners using the `score-flyio provisioners ..` subcommands:

- `score-flyio provisioners list` - lists the configured provisioners in order
- `score-flyio provisioners add ..` - adds a new provisioner configuration to the top of the list
- `score-flyio provisioners remove` - removes a provisioner from the list

The matching logic when provisioner a resource is simple: the CLI will iterate through the list in order and pick the first provisioner that has a resource type, class, and id that matches the subject resource.

You can configure a static provisioner using `--static-json` for example:

```
score-flyio provisioners add environment default-environment --static-json='{"LOG_LEVEL":"DEBUG"}'
```

Note that static provisioners do not support secrets since this would result in the secrets being stored in the project state file which we want to avoid. Use a `cmd` provisioner instead with a more secure script file if needed.

You can configure a static provisioner using `--cmd-binary` and `--cmd-args`:

```
score-flyio provisioners add postgres default-postgres --cmd-binary=python3 --cmd-args=${HOME}/bin/default-postgres-provisioner,'$SCORE_PROVISIONER_MODE'
```

This will execute the binary with the given comma separated args replacing any `$SCORE_PROVISIONER_MODE` with the provisioning mode ("provision" or "deprovision"). For example the above provisioner will end up executing `/usr/local/bin/python3 /home/my-user/bin/default-postgres-provisioner provision` with the resource state passed as input and the output decoded as resource outputs (see below). When cleaning up or destroying the resource, the CLI replaces the last argument with `"deprovision"`. The `$SCORE_PROVISIONER_MODE` environment variable will also be set in the executing context.

Finally, you can configure a remote provisioner using `--http-url`. The CLI will perform an `HTTP POST` request to this URL with the resource inputs passed as the request body and will expect the response body to match the resource outputs schema (see below). The CLI will use an `HTTP DELETE` method when cleaning up or destroying a resource created by a `cmd` provisioner.

#### Resource Inputs Schema

```
application/json
{
    "resource_type": "",
    "resource_class": "",
    "resource_id": "",
    "resource_uid": "",
    "resource_params": {},
    "resource_metadata": {},
    "state": {},
    "shared": {}
}
```

#### Resource Outputs Schema

```
application/json
{
    "state": {},
    "values": {},
    "secrets": {},
    "shared": {}
}
```

### Resource example: configuring the environment stage using a provisioner

Your app may want to know what "stage" it is deployed into and what level to set its log output to. You can create a static environment with this content:

```
score-flyio provisioners add environment default-environment --static-json='{"STAGE":"DEV","LOG_LEVEL":"DEBUG"}'
```

And then consume it in your Score workload:

```yaml
apiVersion: score.dev/v1b1
metadata:
  name: sample
containers:
  main:
    image: my-image
    variables:
      STAGE: ${resources.env.stage}
      LOG_LEVEL: ${resources.env.LOG_LEVEL}
resources:
  env:
    type: environment
```

### Resource example: pulling secrets from 1password

You might also use a password manager such as 1password for storing database credentials for an environment. You could use a `cmd` provisioner to provide these at `generate` time:

```
score-flyio provisioners add postgres prod-postgres --cmd-binary=op --cmd-args=read,op://Private/prod-database-resource/outputs
```

This will pull the `outputs` field out of the `prod-database-resource` item in the `Private` vault of the local 1password which might look like:

```
{
  "values": {
     "host": "/cloudsql/my-project-id:region:myinstanceid",
     "port": "",
     "name": "dbname",
     "user": "username"
  },
  "secrets": {
     "password": "password"
  }
}
```

In this example, we don't need to use the `$SCORE_PROVISIONER_MODE` variable, because the state is static, but a more complex script may need to use this to determine if it is creating or destroying the resource.

### Resource example: using the built-in Fly.io postgres provisioners

We've included a built-in `cmd` provisioner for a [Fly.io-based Postgres](https://fly.io/docs/postgres/). This is experimental and is used to demonstrate how to use asynchronous cmd provisioners that have remote state. This comes in two variants, one for the `postgres` database type and one for the `postgres-instance` which can return a super-user.

You can set this up via:

```
score-flyio provisioners add flypg postgres --cmd-binary=score-flyio --cmd-args='builtin-provisioners,postgres,$SCORE_PROVISIONER_MODE'
score-flyio provisioners add flypginstance postgres-instance --cmd-binary=score-flyio --cmd-args='builtin-provisioners,postgres-instance,$SCORE_PROVISIONER_MODE'
```

You will also need to export a Fly API Token and preferred region as environment variables `FLY_API_TOKEN` and `FLY_REGION_NAME`.

```
export FLY_REGION_NAME=lhr
export FLY_API_TOKEN=$(fly tokens create org -x '24h' -o personal)
```

Then you can use the `postgres` resource type:

```yaml
resources:
    db:
        type: postgres
```

This outputs `host`, `port`, `database`, `username`, and `password` outputs. You can connect to the following connection string from your app:

```yaml
DB: postgres://${resources.db.username}:${resources.db.password}@${resources.db.host}:${resources.db.port}/${resources.db.database}
```

The database starts as a single-node 1cpu cluster but this can be scaled vertically and horizontally using the [guide in the documentation](https://fly.io/docs/postgres/managing/).

Once you have tested this, remember to deprovision the database resource through `score-flyio resources deprovision postgres.default#example.db`.

You can use the following Score file as a test example:

```yaml
apiVersion: score.dev/v1b1
metadata:
    name: example
    annotations:
        score-flyio.astromechza.github.com/service-web-handlers: "tls,http"
        score-flyio.astromechza.github.com/service-web-auto-stop: "stop"
containers:
    main:
        image: ghcr.io/astromechza/demo-app:latest
        variables:
            OVERRIDE_POSTGRES: postgres://${resources.db.username}:${resources.db.password}@${resources.db.host}:${resources.db.port}/${resources.db.database}
        resources:
            requests:
                cpu: "1"
                memory: "128M"
service:
    ports:
        web:
            port: 443
            targetPort: 8080
resources:
    db:
        type: postgres
```
