package jobs

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TypeGenericJob = "jobs:generic"

type QueuePayload struct {
	JobID string `json:"job_id"`
}

type AsynqQueue struct {
	client *asynq.Client
}

func NewAsynqQueue(redisOpt asynq.RedisClientOpt) *AsynqQueue {
	return &AsynqQueue{
		client: asynq.NewClient(redisOpt),
	}
}

func NewAsynqQueueWithClient(client *asynq.Client) *AsynqQueue {
	return &AsynqQueue{client: client}
}

func MarshalQueuePayload(jobID string) ([]byte, error) {
	return json.Marshal(QueuePayload{JobID: jobID})
}

func UnmarshalQueuePayload(payload []byte) (QueuePayload, error) {
	var decoded QueuePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return QueuePayload{}, err
	}
	return decoded, nil
}

func (q *AsynqQueue) Enqueue(ctx context.Context, task *asynq.Task) error {
	_, err := q.client.EnqueueContext(ctx, task)
	return err
}

func (q *AsynqQueue) Close() error {
	if q == nil || q.client == nil {
		return nil
	}
	return q.client.Close()
}
