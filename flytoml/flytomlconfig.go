package flytoml

// Config wraps the properties of app configuration.
// NOTE: If you any new setting here, please also add a value for it at testdata/rull-reference.toml
type Config struct {
	AppName        string    `toml:"app,omitempty" json:"app,omitempty"`
	PrimaryRegion  string    `toml:"primary_region,omitempty" json:"primary_region,omitempty"`
	KillSignal     *string   `toml:"kill_signal,omitempty" json:"kill_signal,omitempty"`
	KillTimeout    *Duration `toml:"kill_timeout,omitempty" json:"kill_timeout,omitempty"`
	SwapSizeMB     *int      `toml:"swap_size_mb,omitempty" json:"swap_size_mb,omitempty"`
	ConsoleCommand string    `toml:"console_command,omitempty" json:"console_command,omitempty"`

	// Sections that are typically short and benefit from being on top
	Experimental *Experimental     `toml:"experimental,omitempty" json:"experimental,omitempty"`
	Build        *Build            `toml:"build,omitempty" json:"build,omitempty"`
	Deploy       *Deploy           `toml:"deploy,omitempty" json:"deploy,omitempty"`
	Env          map[string]string `toml:"env,omitempty" json:"env,omitempty"`

	// Fields that are process group aware must come after Processes
	Processes        map[string]string         `toml:"processes,omitempty" json:"processes,omitempty"`
	Mounts           []Mount                   `toml:"mounts,omitempty" json:"mounts,omitempty"`
	HTTPService      *HTTPService              `toml:"http_service,omitempty" json:"http_service,omitempty"`
	Services         []Service                 `toml:"services,omitempty" json:"services,omitempty"`
	Checks           map[string]*ToplevelCheck `toml:"checks,omitempty" json:"checks,omitempty"`
	Files            []File                    `toml:"files,omitempty" json:"files,omitempty"`
	HostDedicationID string                    `toml:"host_dedication_id,omitempty" json:"host_dedication_id,omitempty"`

	Compute []*Compute `toml:"vm,omitempty" json:"vm,omitempty"`

	// Others, less important.
	Statics []Static   `toml:"statics,omitempty" json:"statics,omitempty"`
	Metrics []*Metrics `toml:"metrics,omitempty" json:"metrics,omitempty"`
}

type Deploy struct {
	ReleaseCommand        string    `toml:"release_command,omitempty" json:"release_command,omitempty"`
	ReleaseCommandTimeout *Duration `toml:"release_command_timeout,omitempty" json:"release_command_timeout,omitempty"`
	Strategy              string    `toml:"strategy,omitempty" json:"strategy,omitempty"`
	MaxUnavailable        *float64  `toml:"max_unavailable,omitempty" json:"max_unavailable,omitempty"`
	WaitTimeout           *Duration `toml:"wait_timeout,omitempty" json:"wait_timeout,omitempty"`
}

type File struct {
	GuestPath  string   `toml:"guest_path" json:"guest_path,omitempty" validate:"required"`
	LocalPath  string   `toml:"local_path,omitempty" json:"local_path,omitempty"`
	SecretName string   `toml:"secret_name,omitempty" json:"secret_name,omitempty"`
	RawValue   string   `toml:"raw_value,omitempty" json:"raw_value,omitempty"`
	Processes  []string `json:"processes,omitempty" toml:"processes,omitempty"`
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path,omitempty" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix,omitempty" validate:"required"`
}

type Mount struct {
	Source                  string   `toml:"source,omitempty" json:"source,omitempty"`
	Destination             string   `toml:"destination,omitempty" json:"destination,omitempty"`
	InitialSize             string   `toml:"initial_size,omitempty" json:"initial_size,omitempty"`
	Processes               []string `toml:"processes,omitempty" json:"processes,omitempty"`
	AutoExtendSizeThreshold int      `toml:"auto_extend_size_threshold,omitempty" json:"auto_extend_size_threshold,omitempty"`
	AutoExtendSizeIncrement string   `toml:"auto_extend_size_increment,omitempty" json:"auto_extend_size_increment,omitempty"`
	AutoExtendSizeLimit     string   `toml:"auto_extend_size_limit,omitempty" json:"auto_extend_size_limit,omitempty"`
}

type Build struct {
	Builder           string            `toml:"builder,omitempty" json:"builder,omitempty"`
	Args              map[string]string `toml:"args,omitempty" json:"args,omitempty"`
	Buildpacks        []string          `toml:"buildpacks,omitempty" json:"buildpacks,omitempty"`
	Image             string            `toml:"image,omitempty" json:"image,omitempty"`
	Settings          map[string]any    `toml:"settings,omitempty" json:"settings,omitempty"`
	Builtin           string            `toml:"builtin,omitempty" json:"builtin,omitempty"`
	Dockerfile        string            `toml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Ignorefile        string            `toml:"ignorefile,omitempty" json:"ignorefile,omitempty"`
	DockerBuildTarget string            `toml:"build-target,omitempty" json:"build-target,omitempty"`
}

type Experimental struct {
	Cmd          []string `toml:"cmd,omitempty" json:"cmd,omitempty"`
	Entrypoint   []string `toml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Exec         []string `toml:"exec,omitempty" json:"exec,omitempty"`
	AutoRollback bool     `toml:"auto_rollback,omitempty" json:"auto_rollback,omitempty"`
	EnableConsul bool     `toml:"enable_consul,omitempty" json:"enable_consul,omitempty"`
	EnableEtcd   bool     `toml:"enable_etcd,omitempty" json:"enable_etcd,omitempty"`
}

type Compute struct {
	Size             string   `json:"size,omitempty" toml:"size,omitempty"`
	Memory           string   `json:"memory,omitempty" toml:"memory,omitempty"`
	CPUKind          string   `json:"cpu_kind,omitempty" toml:"cpu_kind,omitempty"`
	CPUs             int      `json:"cpus,omitempty" toml:"cpus,omitempty"`
	MemoryMB         int      `json:"memory_mb,omitempty" toml:"memory_mb,omitempty"`
	GPUKind          string   `json:"gpu_kind,omitempty" toml:"gpu_kind,omitempty"`
	HostDedicationID string   `json:"host_dedication_id,omitempty" toml:"host_dedication_id,omitempty"`
	KernelArgs       []string `json:"kernel_args,omitempty" toml:"kernel_args,omitempty"`
	Processes        []string `json:"processes,omitempty" toml:"processes,omitempty"`
}

type Service struct {
	Protocol     string `json:"protocol,omitempty" toml:"protocol"`
	InternalPort int    `json:"internal_port,omitempty" toml:"internal_port"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML. The encoder
	// already omits nil since it can't be represented, and omitempty makes it omit false as well.
	AutoStopMachines   *bool                      `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                      `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                       `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Ports              []MachinePort              `json:"ports,omitempty" toml:"ports"`
	Concurrency        *MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	TCPChecks          []*ServiceTCPCheck         `json:"tcp_checks,omitempty" toml:"tcp_checks,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck        `json:"http_checks,omitempty" toml:"http_checks,omitempty"`
	Processes          []string                   `json:"processes,omitempty" toml:"processes,omitempty"`
}

type ServiceTCPCheck struct {
	Interval    *Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval    *Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`

	// HTTP Specifics
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPTLSServerName *string           `json:"tls_server_name,omitempty" toml:"tls_server_name,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

type HTTPService struct {
	InternalPort int  `json:"internal_port,omitempty" toml:"internal_port,omitempty" validate:"required,numeric"`
	ForceHTTPS   bool `toml:"force_https,omitempty" json:"force_https,omitempty"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML; see the note in Service.
	AutoStopMachines   *bool                      `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                      `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                       `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Processes          []string                   `json:"processes,omitempty" toml:"processes,omitempty"`
	Concurrency        *MachineServiceConcurrency `toml:"concurrency,omitempty" json:"concurrency,omitempty"`
	TLSOptions         *TLSOptions                `json:"tls_options,omitempty" toml:"tls_options,omitempty"`
	HTTPOptions        *HTTPOptions               `json:"http_options,omitempty" toml:"http_options,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck        `json:"checks,omitempty" toml:"checks,omitempty"`
}

type Metrics struct {
	*MachineMetrics
	Processes []string `json:"processes,omitempty" toml:"processes,omitempty"`
}

type MachineServiceConcurrency struct {
	Type      string `json:"type,omitempty" toml:"type,omitempty"`
	HardLimit int    `json:"hard_limit,omitempty" toml:"hard_limit,omitempty"`
	SoftLimit int    `json:"soft_limit,omitempty" toml:"soft_limit,omitempty"`
}

type MachineMetrics struct {
	Port int    `toml:"port" json:"port,omitempty"`
	Path string `toml:"path" json:"path,omitempty"`
}

type TLSOptions struct {
	ALPN              []string `json:"alpn,omitempty" toml:"alpn,omitempty"`
	Versions          []string `json:"versions,omitempty" toml:"versions,omitempty"`
	DefaultSelfSigned *bool    `json:"default_self_signed,omitempty" toml:"default_self_signed,omitempty"`
}

type HTTPOptions struct {
	Compress  *bool                `json:"compress,omitempty" toml:"compress,omitempty"`
	Response  *HTTPResponseOptions `json:"response,omitempty" toml:"response,omitempty"`
	H2Backend *bool                `json:"h2_backend,omitempty" toml:"h2_backend,omitempty"`
}

type HTTPResponseOptions struct {
	Headers map[string]any `json:"headers,omitempty" toml:"headers,omitempty"`
}

type MachinePort struct {
	Port              *int               `json:"port,omitempty" toml:"port,omitempty"`
	StartPort         *int               `json:"start_port,omitempty" toml:"start_port,omitempty"`
	EndPort           *int               `json:"end_port,omitempty" toml:"end_port,omitempty"`
	Handlers          []string           `json:"handlers,omitempty" toml:"handlers,omitempty"`
	ForceHTTPS        bool               `json:"force_https,omitempty" toml:"force_https,omitempty"`
	TLSOptions        *TLSOptions        `json:"tls_options,omitempty" toml:"tls_options,omitempty"`
	HTTPOptions       *HTTPOptions       `json:"http_options,omitempty" toml:"http_options,omitempty"`
	ProxyProtoOptions *ProxyProtoOptions `json:"proxy_proto_options,omitempty" toml:"proxy_proto_options,omitempty"`
}

type ProxyProtoOptions struct {
	Version string `json:"version,omitempty" toml:"version,omitempty"`
}

type ToplevelCheck struct {
	Port              *int              `json:"port,omitempty" toml:"port,omitempty"`
	Type              *string           `json:"type,omitempty" toml:"type,omitempty"`
	Interval          *Duration         `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *Duration         `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod       *Duration         `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPTLSServerName *string           `json:"tls_server_name,omitempty" toml:"tls_server_name,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
	Processes         []string          `json:"processes,omitempty" toml:"processes,omitempty"`
}
