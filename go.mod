module github.com/score-spec/score-implementation-sample

go 1.23.4

require (
	github.com/imdario/mergo v1.0.1
	github.com/score-spec/score-go v1.8.5
	github.com/spf13/cobra v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.10.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/imdario/mergo => dario.cat/mergo v1.0.0
