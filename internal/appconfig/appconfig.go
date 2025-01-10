package appconfig

// AppConfig is the Fly application config usually serialised to toml.
// The properties are generally defined in https://github.com/superfly/flyctl/blob/master/internal/appconfig/config.go#L38 but
// we only support a subset of these.
type AppConfig struct {
	AppName      string                   `toml:"app,omitempty" json:"app,omitempty"`
	Build        *Build                   `toml:"build,omitempty" json:"build,omitempty"`
	Checks       map[string]TopLevelCheck `toml:"checks,omitempty" json:"checks,omitempty"`
	Env          map[string]string        `toml:"env,omitempty" json:"env,omitempty"`
	Experimental *Experimental            `toml:"experimental,omitempty" json:"experimental,omitempty"`
	Files        []File                   `toml:"files,omitempty" json:"files,omitempty"`
	Mounts       []Mount                  `toml:"mounts,omitempty" json:"mounts,omitempty"`
	Services     []Service                `toml:"services,omitempty" json:"services,omitempty"`
	Vm           *Vm                      `toml:"vm,omitempty" json:"vm,omitempty"`
}

type Build struct {
	Dockerfile string `toml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	IgnoreFile string `toml:"ignorefile,omitempty" json:"ignorefile,omitempty"`
	Image      string `toml:"image,omitempty" json:"image,omitempty"`
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
	Destination string `toml:"destination,omitempty" json:"destination,omitempty"`
	Source      string `toml:"source,omitempty" json:"source,omitempty"`
}

type File struct {
	GuestPath string  `toml:"guest_path,omitempty" json:"guest_path,omitempty"`
	LocalPath *string `toml:"local_path,omitempty" json:"local_path,omitempty"`
	RawValue  *string `toml:"raw_value,omitempty" json:"raw_value,omitempty"`
}

type Service struct {
	AutoStopMachines   string                 `toml:"auto_stop_machines,omitempty" json:"auto_stop_machines,omitempty"`
	Concurrency        map[string]interface{} `toml:"concurrency,omitempty" json:"concurrency,omitempty"`
	HttpChecks         []HttpCheck            `toml:"http_checks,omitempty" json:"http_checks,omitempty"`
	InternalPort       int                    `toml:"internal_port,omitempty" json:"internal_port,omitempty"`
	MinMachinesRunning int                    `toml:"min_machines_running,omitempty" json:"min_machines_running,omitempty"`
	Ports              []ServicePort          `toml:"ports,omitempty" json:"ports,omitempty"`
	Protocol           string                 `toml:"protocol,omitempty" json:"protocol,omitempty"`
}

type ServicePort struct {
	Handlers    []string               `toml:"handlers,omitempty" json:"handlers,omitempty"`
	HttpOptions map[string]interface{} `toml:"http_options,omitempty" json:"http_options,omitempty"`
	Port        int                    `toml:"port,omitempty" json:"port,omitempty"`
}

type HttpCheck struct {
	Headers       map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
	Method        string            `toml:"method,omitempty" json:"method,omitempty"`
	Path          string            `toml:"path,omitempty" json:"path,omitempty"`
	Protocol      string            `toml:"protocol,omitempty" json:"protocol,omitempty"`
	TlsServerName string            `toml:"tls_server_name,omitempty" json:"tls_server_name,omitempty"`
	TlsSkipVerify bool              `toml:"tls_skip_verify,omitempty" json:"tls_skip_verify,omitempty"`
}

type TopLevelCheck struct {
	Headers map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
	Method  string            `toml:"method,omitempty" json:"method,omitempty"`
	Path    string            `toml:"path,omitempty" json:"path,omitempty"`
	Port    int               `toml:"port,omitempty" json:"port,omitempty"`
	Type    string            `toml:"type,omitempty" json:"type,omitempty"`
}
