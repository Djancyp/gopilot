package ui

import "time"

// MessageType represents the type of message
type MessageType int

const (
	UserMessage MessageType = iota
	AssistantMessage
	SystemMessage
	ErrorMessage
)

// Message represents a chat message
type Message struct {
	Type      MessageType
	Content   string
	Timestamp time.Time
}

// NewMessage creates a new message with the current timestamp
func NewMessage(msgType MessageType, content string) Message {
	return Message{
		Type:      msgType,
		Content:   content,
		Timestamp: time.Now(),
	}
}
