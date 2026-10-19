package runtime

import (
	"context"
	"io"
	"maps"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ewhauser/gbash/commands"
	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/host"
	"github.com/ewhauser/gbash/internal/builtins"
	"github.com/ewhauser/gbash/network"
	"github.com/ewhauser/gbash/policy"
	"github.com/ewhauser/gbash/shell/analysis"
)

type Config struct {
	FileSystem       FileSystemConfig
	Registry         commands.CommandRegistry
	Policy           policy.Policy
	LimitOverrides   policy.Limits
	BaseEnv          map[string]string
	Host             host.Adapter
	Network          *network.Config
	NetworkClient    network.Client
	Tracing          TraceConfig
	Logger           LogCallback
	AnalysisObserver analysis.Observer
	CompatHostRoots  []string
}

type Runtime struct {
	cfg            Config
	sessionFactory sessionFactory
}

type Session struct {
	cfg         Config
	id          string
	fs          gbfs.FileSystem
	bootAt      time.Time
	currentTime time.Time
	clockRealAt time.Time
	layout      *sandboxLayoutState
	mu          sync.Mutex
	clockMu     sync.RWMutex
}

func New(opts ...Option) (*Runtime, error) {
	resolved, err := resolveConfig(opts)
	if err != nil {
		return nil, err
	}
	defaultSessionFS := resolved.FileSystem.Factory == nil
	resolved.FileSystem = resolved.FileSystem.resolved()
	if resolved.Registry == nil {
		resolved.Registry = builtins.DefaultRegistry()
	}
	if resolved.NetworkClient == nil && resolved.Network != nil {
		client, err := network.New(resolved.Network)
		if err != nil {
			return nil, err
		}
		resolved.NetworkClient = client
	}
	if resolved.NetworkClient != nil {
		if err := builtins.EnsureNetworkCommands(resolved.Registry); err != nil {
			return nil, err
		}
	}
	if resolved.Policy == nil {
		resolved.Policy = policy.NewStatic(&policy.Config{
			AllowedCommands: resolved.Registry.Names(),
			ReadRoots:       []string{"/"},
			WriteRoots:      []string{"/"},
			Limits:          effectiveLimits(policy.Limits{}, resolved.LimitOverrides),
			SymlinkMode:     policy.SymlinkDeny,
		})
	} else {
		resolved.Policy = &limitDefaultsPolicy{
			base:      resolved.Policy,
			overrides: resolved.LimitOverrides,
		}
	}
	if resolved.Host == nil {
		resolved.Host = newVirtualHost()
	}
	hostEnv, err := runtimeBaseEnv(context.Background(), resolved.Host)
	if err != nil {
		return nil, err
	}
	resolved.BaseEnv = mergeEnv(hostEnv, resolved.BaseEnv)

	factory := sessionFactory(plainSessionFactory{base: resolved.FileSystem.Factory})
	if defaultSessionFS {
		factory = &preparedMemorySessionFactory{
			base:     resolved.FileSystem.Factory,
			env:      resolved.BaseEnv,
			workDir:  resolved.FileSystem.WorkingDir,
			commands: resolved.Registry.Names(),
		}
	}

	return &Runtime{
		cfg:            resolved,
		sessionFactory: factory,
	}, nil
}

// defaultLimits are the runtime's resource limits. They apply to the default
// policy and fill every zero-valued field of a custom policy's Limits.
func defaultLimits() policy.Limits {
	return policy.Limits{
		MaxCommandCount:        10000,
		MaxGlobOperations:      100000,
		MaxBraceExpansionWords: 100000,
		MaxLoopIterations:      10000,
		MaxSubstitutionDepth:   50,
		MaxStdoutBytes:         1 << 20,
		MaxStderrBytes:         1 << 20,
		MaxFileBytes:           8 << 20,
		MaxVariableBytes:       32 << 20,
	}
}

// effectiveLimits layers a policy's limits and the runtime overrides over
// defaultLimits. Zero fields inherit the layer below; negative fields
// (policy.Unlimited) disable that limit and are reported as zero, which is
// how limit consumers spell "no limit".
func effectiveLimits(base, overrides policy.Limits) policy.Limits {
	return normalizedLimits(mergedLimits(mergedLimits(defaultLimits(), base), overrides))
}

// limitDefaultsPolicy wraps a custom policy so that limits it leaves at zero
// fall back to the runtime defaults instead of silently disabling the limit.
// Limits are resolved on every call so dynamic policies keep working.
type limitDefaultsPolicy struct {
	base      policy.Policy
	overrides policy.Limits
}

func (p *limitDefaultsPolicy) AllowCommand(ctx context.Context, name string, argv []string) error {
	return p.base.AllowCommand(ctx, name, argv)
}

func (p *limitDefaultsPolicy) AllowBuiltin(ctx context.Context, name string, argv []string) error {
	return p.base.AllowBuiltin(ctx, name, argv)
}

func (p *limitDefaultsPolicy) AllowPath(ctx context.Context, action policy.FileAction, target string) error {
	return p.base.AllowPath(ctx, action, target)
}

func (p *limitDefaultsPolicy) Limits() policy.Limits {
	return effectiveLimits(p.base.Limits(), p.overrides)
}

func (p *limitDefaultsPolicy) SymlinkMode() policy.SymlinkMode {
	return p.base.SymlinkMode()
}

// mergedLimits returns base with every non-zero field of overrides applied.
// It walks the struct reflectively so new policy.Limits fields are merged
// without having to be listed here.
func mergedLimits(base, overrides policy.Limits) policy.Limits {
	dst := reflect.ValueOf(&base).Elem()
	src := reflect.ValueOf(overrides)
	for i := range src.NumField() {
		if field := src.Field(i); !field.IsZero() {
			dst.Field(i).Set(field)
		}
	}
	return base
}

// normalizedLimits maps negative (disabled) integer limits to zero.
func normalizedLimits(limits policy.Limits) policy.Limits {
	for _, field := range reflect.ValueOf(&limits).Elem().Fields() {
		if field.CanInt() && field.Int() < 0 {
			field.SetInt(0)
		}
	}
	return limits
}

func (r *Runtime) NewSession(ctx context.Context) (*Session, error) {
	fsys, err := r.sessionFactory.New(ctx)
	if err != nil {
		return nil, err
	}
	fsys = wrapSandboxFileSystem(fsys)

	if !r.sessionFactory.layoutReady() {
		if err := initializeSandboxLayout(ctx, fsys, r.cfg.BaseEnv, r.cfg.FileSystem.WorkingDir, r.cfg.Registry.Names()); err != nil {
			return nil, err
		}
	}

	now := time.Now()
	return &Session{
		cfg:         r.cfg,
		id:          nextTraceID("sess"),
		fs:          fsys,
		bootAt:      now.UTC(),
		currentTime: now.UTC(),
		clockRealAt: now,
		layout:      newSandboxLayoutState(r.cfg.BaseEnv, r.cfg.FileSystem.WorkingDir),
	}, nil
}

func (r *Runtime) Run(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	session, err := r.NewSession(ctx)
	if err != nil {
		return nil, err
	}
	return session.Exec(ctx, req)
}

func defaultName(name string) string {
	if name == "" {
		return "stdin"
	}
	return name
}

func mergeEnv(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func stdinOrEmpty(reader io.Reader) io.Reader {
	if reader == nil {
		return strings.NewReader("")
	}
	return reader
}
