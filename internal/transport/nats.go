package transport

import (
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const DefaultConnectionTimeout = 10 * time.Second

func Connect(natsURL string, opts ...nats.Option) (*nats.Conn, nats.JetStreamContext, error) {
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	defaultOpts := []nats.Option{
		nats.Timeout(DefaultConnectionTimeout),
		nats.ReconnectWait(1 * time.Second),
		nats.MaxReconnects(-1),
	}
	defaultOpts = append(defaultOpts, opts...)

	nc, err := nats.Connect(natsURL, defaultOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}
	return nc, js, nil
}

func TasksStreamName(projectID string) string { // normalized stream name for a project
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, projectID)
	return fmt.Sprintf("TASKS_%s", sanitized)
}

func TasksSubject(projectID string) string { // subject used for publish/consume
	return fmt.Sprintf("tasks.%s", projectID)
}

func TaskRequestSubject(projectID string) string {
	return fmt.Sprintf("requests.%s", projectID)
}

func ResultsSubject(projectID string) string {
	return fmt.Sprintf("results.%s", projectID)
}

func AnnouncementsSubject() string { // subject for global project announcements
	return "announcements"
}

func ProjectInfoSubject(projectID string) string { // subject for querying specific project information
	return fmt.Sprintf("projects.%s.info", projectID)
}

// creates or refreshes a JetStream WorkQueue stream
func EnsureTaskStream(js nats.JetStreamContext, projectID string) (*nats.StreamInfo, error) {
	streamName := TasksStreamName(projectID)
	subj := TasksSubject(projectID)
	streamCfg := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subj},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.MemoryStorage,
		MaxAge:    24 * time.Hour,
	}

	info, err := js.StreamInfo(streamName)
	if err == nil && info != nil { // Delete existing stream to ensure clean WorkQueue state for new run
		_ = js.DeleteStream(streamName)
	}
	info, err = js.AddStream(streamCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream stream %s: %w", streamName, err)
	}
	return info, nil
}

func PublishTask(js nats.JetStreamContext, projectID string, data []byte) error {
	subj := TasksSubject(projectID)
	_, err := js.Publish(subj, data)
	if err != nil {
		return fmt.Errorf("failed to publish task to %s: %w", subj, err)
	}
	return nil
}
