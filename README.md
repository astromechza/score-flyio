# score-flyio

| ⚠️ This project is still being developed! Track the development here.

A Score transformer for Fly.io. Convert your Score application files into Fly.io machines and apps!

## Usage

The following command will validate and transform the Score file into a Fly.io Machine configuration and deploy or update it.

```
$ go install github.com/astromechza/score-flyio@latest
go: downloading github.com/astromechza/score-flyio v0.0.0-20231206214427-f5eb613bc02b

$ which score-flyio
/Users/bmeier/.gvm/pkgsets/go1.21.0/global/bin/score-flyio

$ score-flyio deploy --app score-flyio-1234 examples/01-hello-world.yaml
```

Any container volumes will be converted into per-machine Fly.io volumes. Unless they are defined in the resources section as volumes when they will be provisioned as shared volumes.

Any services will be converted into Fly.io Machine services.

A DNS resource will be converted into a Fly.io shared ipv4 address.

Live-ness and readiness probes will be converted into checks as appropriate.

Files will be converted into Fly.io files.

If a Postgres resource is added, and it has an annotation pointing to an existing Fly.io Postgres app within the same org, we will "attach" it to the Fly application via the DATABASE_URL environment variable.
