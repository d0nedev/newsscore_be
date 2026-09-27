package metrics

import (
	"context"
	"fmt"

	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Config struct {
	ServiceName string
	Version     string
	Environment string
	Endpoint    string
	Insecure    bool
}

func Init(cfg Config) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(tracing.Resource(cfg.ServiceName, cfg.Version, cfg.Environment)),
	)

	otel.SetMeterProvider(provider)

	return provider, nil
}

func Shutdown(ctx context.Context, provider *sdkmetric.MeterProvider) error {
	return provider.Shutdown(ctx)
}
