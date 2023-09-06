package core

import (
	"net/http"
	"testing"
)

func TestGetProjectByBadToken(t *testing.T) {
	client := NewQodanaClient()
	result := client.GetProjectByToken("https://www.jetbrains.com")
	switch v := result.(type) {
	case Success:
		t.Errorf("Did not expect request error: %v", v)
	case APIError:
		if v.StatusCode > http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, v.StatusCode)
		}
	case RequestError:
		t.Errorf("Did not expect request error: %v", v)
	default:
		t.Error("Unknown result type")
	}
}

func TestValidateToken(t *testing.T) {
	client := NewQodanaClient()
	if projectName := client.validateToken("kek"); projectName != "" {
		t.Errorf("Problem")
	}
}
