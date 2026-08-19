package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

type Checker interface {
	Check(context.Context, domain.Source) domain.Observation
}

type Registry struct {
	checkers map[domain.SourceKind]Checker
}

func NewRegistry() *Registry {
	return &Registry{checkers: map[domain.SourceKind]Checker{
		domain.SourceHTTP:      HTTP{},
		domain.SourceTCP:       TCP{},
		domain.SourceDNS:       DNS{},
		domain.SourceICMP:      ICMP{},
		domain.SourceHeartbeat: Heartbeat{},
	}}
}

func (registry *Registry) Check(ctx context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	if err := source.Validate(); err != nil {
		return unknown(source, startedAt, "invalid_source", err)
	}
	checker, ok := registry.checkers[source.Kind]
	if !ok {
		return unknown(source, startedAt, "unsupported_source", fmt.Errorf("source kind %q is not registered", source.Kind))
	}
	return checker.Check(ctx, source)
}

func healthy(source domain.Source, startedAt time.Time) domain.Observation {
	return domain.NewObservation(source, domain.OutcomeHealthy, startedAt)
}

func unhealthy(source domain.Source, startedAt time.Time, reason string, err error) domain.Observation {
	observation := domain.NewObservation(source, domain.OutcomeUnhealthy, startedAt)
	observation.Reason = reason
	observation.Message = err.Error()
	return observation
}

func unknown(source domain.Source, startedAt time.Time, reason string, err error) domain.Observation {
	observation := domain.NewObservation(source, domain.OutcomeUnknown, startedAt)
	observation.Reason = reason
	observation.Message = err.Error()
	return observation
}
