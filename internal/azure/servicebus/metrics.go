package servicebus

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
)

// EntityCounts holds the current active and dead-letter message counts
// for one queue, topic, or subscription as reported by Azure Monitor.
type EntityCounts struct {
	EntityName      string
	ActiveMsgCount  int64
	DeadLetterCount int64
}

var newMetricsClient = azquery.NewMetricsClient

func (s *Service) getMetricsClient() (*azquery.MetricsClient, error) {
	for {
		s.mu.Lock()
		if s.metricsClient != nil {
			s.mu.Unlock()
			return s.metricsClient, nil
		}
		cred := s.cred
		generation := s.generation
		s.mu.Unlock()

		client, err := newMetricsClient(cred, nil)
		if err != nil {
			return nil, fmt.Errorf("create metrics client: %w", err)
		}

		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			continue
		}
		if s.metricsClient != nil {
			cached := s.metricsClient
			s.mu.Unlock()
			return cached, nil
		}
		s.metricsClient = client
		s.mu.Unlock()
		return client, nil
	}
}

// GetNamespaceMetrics queries Azure Monitor for per-entity active and
// dead-letter message counts in the given namespace. Returns one
// EntityCounts per queue/topic. Counts can be up to a few minutes
// stale — Azure Monitor ingests metrics asynchronously.
func (s *Service) GetNamespaceMetrics(ctx context.Context, ns Namespace) ([]EntityCounts, error) {
	client, err := s.getMetricsClient()
	if err != nil {
		return nil, err
	}

	resourceURI := fmt.Sprintf(
		"subscriptions/%s/resourceGroups/%s/providers/Microsoft.ServiceBus/namespaces/%s",
		ns.SubscriptionID, ns.ResourceGroup, ns.Name,
	)

	now := time.Now().UTC()
	resp, err := client.QueryResource(ctx, resourceURI, &azquery.MetricsClientQueryResourceOptions{
		MetricNames: to.Ptr("ActiveMessages,DeadletteredMessages"),
		Timespan:    to.Ptr(azquery.NewTimeInterval(now.Add(-10*time.Minute), now)),
		Interval:    to.Ptr("PT1M"),
		Aggregation: to.SliceOfPtrs(azquery.AggregationTypeAverage),
		Filter:      to.Ptr("EntityName eq '*'"),
	})
	if err != nil {
		return nil, fmt.Errorf("query metrics for %s: %w", ns.Name, err)
	}

	byEntity := make(map[string]*EntityCounts)
	for _, metric := range resp.Value {
		if metric == nil || metric.Name == nil || metric.Name.Value == nil {
			continue
		}
		metricName := *metric.Name.Value
		for _, ts := range metric.TimeSeries {
			if ts == nil {
				continue
			}
			entity := entityNameFromMetadata(ts.MetadataValues)
			if entity == "" {
				continue
			}
			value := latestNonNilAverage(ts.Data)
			if value < 0 {
				continue
			}
			ec, ok := byEntity[entity]
			if !ok {
				ec = &EntityCounts{EntityName: entity}
				byEntity[entity] = ec
			}
			switch metricName {
			case "ActiveMessages":
				ec.ActiveMsgCount = int64(value)
			case "DeadletteredMessages":
				ec.DeadLetterCount = int64(value)
			}
		}
	}

	out := make([]EntityCounts, 0, len(byEntity))
	for _, ec := range byEntity {
		out = append(out, *ec)
	}
	return out, nil
}

func entityNameFromMetadata(values []*azquery.MetadataValue) string {
	for _, mv := range values {
		if mv == nil || mv.Name == nil || mv.Name.Value == nil || mv.Value == nil {
			continue
		}
		if *mv.Name.Value == "entityname" || *mv.Name.Value == "EntityName" {
			return *mv.Value
		}
	}
	return ""
}

// latestNonNilAverage returns the most recent non-nil Average value in
// the time series. Returns -1 when every data point is nil (common
// when the time window has no activity for this entity).
func latestNonNilAverage(points []*azquery.MetricValue) float64 {
	for i := len(points) - 1; i >= 0; i-- {
		p := points[i]
		if p == nil || p.Average == nil {
			continue
		}
		return *p.Average
	}
	return -1
}
