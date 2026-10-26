package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kagent-dev/kagent/go/adk/pkg/a2a"
	kagentagent "github.com/kagent-dev/kagent/go/adk/pkg/agent"
	"github.com/kagent-dev/kagent/go/adk/pkg/app"
	"github.com/kagent-dev/kagent/go/adk/pkg/auth"
	"github.com/kagent-dev/kagent/go/adk/pkg/config"
	"github.com/kagent-dev/kagent/go/adk/pkg/controllerclient"
	"github.com/kagent-dev/kagent/go/adk/pkg/session"
	"github.com/kagent-dev/kagent/go/adk/pkg/telemetry"
	processconfig "github.com/kagent-dev/kagent/go/core/pkg/config"
	"github.com/kagent-dev/kagent/go/pkg/logging"
	"github.com/kagent-dev/kagent/go/pkg/tracing"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/tool/functiontool"
)

type addInput struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addOutput struct {
	Sum int `json:"sum"`
}

func addNumbers(_ adkagent.Context, in addInput) (addOutput, error) {
	return addOutput{Sum: in.A + in.B}, nil
}

func main() {
	// A temp dir is writable in distroless.
	if err := run(filepath.Join(os.TempDir(), "kagent-config")); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(configDir string) error {
	// The controller compiles the AgentTemplate (model, system prompt, MCP servers) and the
	// runtime settings (identity, control-plane URL, listen port, telemetry) into the
	// KAGENT_CONFIG_JSON env var. MaterializeFromEnv writes it to config.json and
	// LoadAgentConfigs parses it. Nothing connects to an MCP server yet.
	if err := config.MaterializeFromEnv(configDir); err != nil {
		return fmt.Errorf("materialize agent config: %w", err)
	}
	compiled, agentCard, err := config.LoadAgentConfigs(configDir)
	if err != nil {
		return fmt.Errorf("load agent config from %s: %w", configDir, err)
	}
	runtime := config.ProcessConfig(compiled)

	logger, err := logging.New(os.Stderr, cmp.Or(runtime.LogLevel, "info"))
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	ctx := logging.IntoContext(context.Background(), logger)

	name, namespace := runtime.Name, runtime.Namespace
	if name == "" || namespace == "" {
		return fmt.Errorf("runtime.name and runtime.namespace are required for trace attribution")
	}
	if runtime.ControlPlaneUrl == "" {
		return fmt.Errorf("runtime.control_plane_url is required: the controller stores this runtime's tasks")
	}
	if agentCard == nil {
		return fmt.Errorf("the compiled config carries no agent card")
	}
	// Same app identity the stock golang-adk runtime derives, so the controller
	// attributes tasks and tokens to this Agent.
	appName := strings.ReplaceAll(namespace, "-", "_") + "__NS__" + strings.ReplaceAll(name, "-", "_")

	// The controller compiles the collector endpoint into runtime.telemetry; the standard
	// OTEL_* SDK variables still override it.
	identity := processconfig.RuntimeTelemetry(runtime.GetTelemetry()).WithEnvironment(os.Environ())
	identity.Runtime = tracing.RuntimeADKGo
	identity.AgentName = name
	identity.AgentNamespace = namespace
	providers, err := telemetry.Init(ctx, identity)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	if !providers.TracesEnabled() {
		return fmt.Errorf("ADK tracing did not initialize: set config.runtimeDefaults.telemetry.traces.endpoint on the kagent-enterprise chart")
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := providers.Shutdown(flushCtx); err != nil {
			logger.Error("flush telemetry on shutdown", "error", err)
		}
	}()

	// One authenticated controller channel. The A2A task store writes each task
	// through it, so the controller can read the turn's result when the stream ends.
	tokenService := auth.NewKAgentTokenService(appName)
	if err := tokenService.Start(ctx); err != nil {
		logger.Error("start token service", "error", err)
	}
	defer tokenService.Stop()
	controller, err := controllerclient.New(controllerclient.Config{
		APIURL: runtime.ControlPlaneUrl, AgentName: appName, TokenProvider: tokenService,
	})
	if err != nil {
		return fmt.Errorf("create controller client for %s: %w", runtime.ControlPlaneUrl, err)
	}
	defer func() {
		if err := controller.Close(); err != nil {
			logger.Error("close controller client", "error", err)
		}
	}()

	addition, err := functiontool.New(functiontool.Config{
		Name: "add_numbers", Description: "Add two integers and return their sum.",
	}, addNumbers)
	if err != nil {
		return fmt.Errorf("create addition tool: %w", err)
	}
	// Build the agent from the template. CreateGoogleADKAgent applies the model and
	// prompt, connects to each MCP server, and exposes its allowed tools. add_numbers
	// is passed as an extra tool, so the agent has both.
	agent, err := kagentagent.CreateGoogleADKAgent(ctx, compiled, strings.ReplaceAll(name, "-", "_"), nil, addition)
	if err != nil {
		return fmt.Errorf("create ADK agent from AgentTemplate: %w", err)
	}

	// session_db_url points at the actor's DurableDir, so ADK sessions survive
	// suspend, resume and actor replacement.
	sessions, err := session.NewService(compiled.Agent.SessionDbUrl)
	if err != nil {
		return fmt.Errorf("open session store %s: %w", compiled.Agent.SessionDbUrl, err)
	}
	runnerConfig := runner.Config{AppName: appName, Agent: agent, SessionService: sessions}
	executor, err := a2a.NewKAgentExecutor(a2a.KAgentExecutorConfig{
		RunnerConfig: runnerConfig, SessionService: sessions, Stream: compiled.Agent.GetStream(),
		AppName: appName, Logger: logger, Flush: providers.ForceFlush, AgentName: name,
	})
	if err != nil {
		return fmt.Errorf("create A2A executor: %w", err)
	}
	// A2A task events stay streamable even when model responses are not.
	agentCard.Capabilities.Streaming = true

	// app.New serves private A2A on runtime.port and GET /readyz on :8081.
	application, err := app.New(app.AppConfig{
		ControllerClient: controller, AgentCard: *agentCard, Host: runtime.Host,
		Port: fmt.Sprint(runtime.GetPort()), MaxContentLength: runtime.MaxContentLength,
		AppName: appName, Logger: logger, Agent: agent, Telemetry: identity,
		Flush: providers.ForceFlush,
	}, executor)
	if err != nil {
		return fmt.Errorf("create A2A server: %w", err)
	}
	return application.Run()
}
