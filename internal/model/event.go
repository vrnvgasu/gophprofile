package model

import "encoding/json"

// EventType — тип события, публикуемого в брокер сообщений.
type EventType string

const (
	EventAvatarUploaded EventType = "avatar.uploaded"
	EventAvatarDeleted  EventType = "avatar.deleted"
)

type AvatarUploadEvent struct {
	AvatarID string `json:"avatar_id"`
	UserID   string `json:"user_id"`
	S3Key    string `json:"s3_key"`
}

type AvatarDeleteEvent struct {
	AvatarID string   `json:"avatar_id"`
	S3Keys   []string `json:"s3_keys"`
}

type Event struct {
	ID      string          `json:"id"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func NewEvent(id string, eventType EventType, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}

	return Event{ID: id, Type: eventType, Payload: raw}, nil
}
