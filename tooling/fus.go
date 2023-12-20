package tooling

import _ "embed"

//go:embed qodana-fuser.jar
var Fuser []byte

type FuserEvent struct {
	GroupId   string            `json:"groupId,omitempty"`
	EventName string            `json:"eventName,omitempty"`
	Time      int64             `json:"time,omitempty"`
	State     bool              `json:"state"`
	EventData map[string]string `json:"eventData,omitempty"`
	SessionId string            `json:"sessionId,omitempty"`
}
