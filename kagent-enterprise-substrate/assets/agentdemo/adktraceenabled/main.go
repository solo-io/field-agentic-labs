package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	a2atype "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/kagent-dev/kagent/go/adk/pkg/a2a"
	"github.com/kagent-dev/kagent/go/adk/pkg/app"
	"github.com/kagent-dev/kagent/go/adk/pkg/models"
	"github.com/kagent-dev/kagent/go/adk/pkg/telemetry"
	"github.com/kagent-dev/kagent/go/pkg/logging"
	"github.com/kagent-dev/kagent/go/pkg/tracing"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
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
	if err := run(); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	ctx := logging.IntoContext(context.Background(), logger)

	name := os.Getenv("KAGENT_NAME")
	namespace := os.Getenv("KAGENT_NAMESPACE")
	if name == "" || namespace == "" {
		return fmt.Errorf("KAGENT_NAME and KAGENT_NAMESPACE are required for trace attribution")
	}
	if os.Getenv("OTEL_TRACING_ENABLED") != "true" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return fmt.Errorf("OTEL_TRACING_ENABLED=true and OTEL_EXPORTER_OTLP_TRACES_ENDPOINT are required")
	}

	identity := tracing.RuntimeTelemetry{
		Runtime: tracing.RuntimeADKGo, AgentName: name, AgentNamespace: namespace,
		Provider: "openai", Model: os.Getenv("MODEL_NAME"),
	}
	shutdown, enabled, err := telemetry.Init(ctx, identity)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	if !enabled {
		return fmt.Errorf("ADK tracing did not initialize")
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := shutdown(flushCtx); err != nil {
			logger.Error("flush telemetry on shutdown", "error", err)
		}
	}()

	modelName := os.Getenv("MODEL_NAME")
	if modelName == "" {
		return fmt.Errorf("MODEL_NAME is required")
	}
	model, err := models.NewOpenAIModel(ctx, &models.OpenAIConfig{Model: modelName})
	if err != nil {
		return fmt.Errorf("create OpenAI model: %w", err)
	}
	addition, err := functiontool.New(functiontool.Config{
		Name: "add_numbers", Description: "Add two integers and return their sum.",
	}, addNumbers)
	if err != nil {
		return fmt.Errorf("create addition tool: %w", err)
	}
	agent, err := llmagent.New(llmagent.Config{
		Name: "adk_trace_enabled", Description: "An agent that demonstrates model and tool spans.",
		Instruction: "Answer briefly. When asked to add two integers, call add_numbers before answering. Report the tool's result.",
		Model:       model, Tools: []tool.Tool{addition},
	})
	if err != nil {
		return fmt.Errorf("create ADK agent: %w", err)
	}

	sessions := adksession.InMemoryService()
	runnerConfig := runner.Config{AppName: name, Agent: agent, SessionService: sessions}
	executor := a2a.NewKAgentExecutor(a2a.KAgentExecutorConfig{
		RunnerConfig: runnerConfig, SessionService: sessions, Stream: true, AppName: name, Logger: logger,
	})
	application, err := app.New(app.AppConfig{
		AgentCard: a2atype.AgentCard{
			Name: name, Description: "Programmatic Go ADK trace demonstration", Version: "1.0.0",
			SupportedInterfaces: []*a2atype.AgentInterface{
				a2atype.NewAgentInterface("http://127.0.0.1:80", a2atype.TransportProtocolGRPC),
			},
			Capabilities:      a2atype.AgentCapabilities{Streaming: true},
			DefaultInputModes: []string{"text"}, DefaultOutputModes: []string{"text"},
		},
		Port: "80", AppName: name, Logger: logger, Agent: agent, Telemetry: identity,
	}, executor)
	if err != nil {
		return fmt.Errorf("create A2A server: %w", err)
	}
	return application.Run()
}
