package appconfig

// AppConfig is the Fly application config usually serialised to toml.
// The properties are generally defined in https://github.com/superfly/flyctl/blob/master/internal/appconfig/config.go#L38 but
// we only support a subset of these.
type AppConfig struct {
	AppName      string            `toml:"app,omitempty" json:"app,omitempty"`
	Build        *Build            `toml:"build,omitempty" json:"build,omitempty"`
	Env          map[string]string `toml:"env,omitempty" json:"env,omitempty"`
	Vm           *Vm               `toml:"vm,omitempty" json:"vm,omitempty"`
	Mounts       []Mount           `toml:"mounts,omitempty" json:"mounts,omitempty"`
	Files        []File            `toml:"files,omitempty" json:"files,omitempty"`
	Experimental *Experimental     `toml:"experimental,omitempty" json:"experimental,omitempty"`
}

type Build struct {
	Image      string `toml:"image,omitempty" json:"image,omitempty"`
	Dockerfile string `toml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	IgnoreFile string `toml:"ignorefile,omitempty" json:"ignorefile,omitempty"`
}

type Experimental struct {
	Cmd        []string `toml:"cmd,omitempty" json:"cmd,omitempty"`
	Entrypoint []string `toml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
}

type Vm struct {
	Cpus   int    `toml:"cpus,omitempty" json:"cpus,omitempty"`
	Memory string `toml:"memory,omitempty" json:"memory,omitempty"`
}

type Mount struct {
	Source      string `toml:"source,omitempty" json:"source,omitempty"`
	Destination string `toml:"destination,omitempty" json:"destination,omitempty"`
}

type File struct {
	GuestPath string  `toml:"guest_path,omitempty" json:"guest_path,omitempty"`
	RawValue  *string `toml:"raw_value,omitempty" json:"raw_value,omitempty"`
	LocalPath *string `toml:"local_path,omitempty" json:"local_path,omitempty"`
}
