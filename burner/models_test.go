package burner

import (
	"testing"
	"time"
)

func TestNewInbox(t *testing.T) {
	i := NewInbox()

	if i.FailedToCreate {
		t.Errorf("TestNewInbox: failed to create not true")
	}

	if i.EmailProviderRouteID != "-" {
		t.Errorf("TestNewInbox: route id not -")
	}
}

func TestGetReceivedDetails(t *testing.T) {
	msgs := []Message{
		{
			InboxID:         "9101112",
			ID:              "5678",
			ReceivedAt:      time.Now().Unix(),
			EmailProviderID: "56789",
			Sender:          "bob@example.com",
			From:            "Bobby Tables <bob@example.com>",
			Subject:         "DELETE FROM MESSAGES;",
			BodyPlain:       "Hello there how are you!",
			BodyHTML:        "<html><body><p>Hello there how are you!</p></body></html>",
			TTL:             time.Now().Add(5 * time.Minute).Unix(),
		},
		{
			InboxID:         "9101112",
			ID:              "9999",
			ReceivedAt:      time.Now().Add(-30 * time.Minute).Unix(),
			EmailProviderID: "56789",
			Sender:          "bob@example.com",
			From:            "Bobby Tables <bob@example.com>",
			Subject:         "DELETE FROM MESSAGES;",
			BodyPlain:       "Hello there how are you!",
			BodyHTML:        "<html><body><p>Hello there how are you!</p></body></html>",
			TTL:             time.Now().Add(5 * time.Minute).Unix(),
		},
		{
			InboxID:         "9101112",
			ID:              "9999",
			ReceivedAt:      time.Now().Add(-10 * time.Second).Add(-30 * time.Minute).Add(-2 * time.Hour).Unix(),
			EmailProviderID: "56789",
			Sender:          "bob@example.com",
			From:            "Bobby Tables <bob@example.com>",
			Subject:         "DELETE FROM MESSAGES;",
			BodyPlain:       "Hello there how are you!",
			BodyHTML:        "<html><body><p>Hello there how are you!</p></body></html>",
			TTL:             time.Now().Add(5 * time.Minute).Unix(),
		},
	}

	details := GetReceivedDetails(msgs)

	if details[0] != "Less than 30s ago" {
		t.Errorf("TestGetReceivedDetails: details[0] incorrect. Should be 'Less than 30s ago' is: %v", details[0])
	}

	if details[1] != "30m ago" {
		t.Errorf("TestGetReceivedDetails: details[1] incorrect. Should be '30m ago' is: %v", details[1])
	}

	if details[2] != "2h 30m ago" {
		t.Errorf("TestGetReceivedDetails: details[2] incorrect. Should be '2h 30m ago' is: %v", details[2])
	}
}
