package sarif

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPropertyBag_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		pb      PropertyBag
		checkFn func(t *testing.T, data []byte)
	}{
		{
			name: "with additional properties",
			pb: PropertyBag{
				AdditionalProperties: map[string]any{
					"key1": "value1",
					"key2": 42,
				},
			},
			checkFn: func(t *testing.T, data []byte) {
				var m map[string]any
				err := json.Unmarshal(data, &m)
				assert.NoError(t, err)
				assert.Equal(t, "value1", m["key1"])
				assert.Equal(t, float64(42), m["key2"])
			},
		},
		{
			name: "with tags",
			pb: PropertyBag{
				Tags: []string{"tag1", "tag2"},
			},
			checkFn: func(t *testing.T, data []byte) {
				var m map[string]any
				err := json.Unmarshal(data, &m)
				assert.NoError(t, err)
				tags := m["tags"].([]any)
				assert.Len(t, tags, 2)
			},
		},
		{
			name: "with both properties and tags",
			pb: PropertyBag{
				AdditionalProperties: map[string]any{"foo": "bar"},
				Tags:                 []string{"mytag"},
			},
			checkFn: func(t *testing.T, data []byte) {
				var m map[string]any
				err := json.Unmarshal(data, &m)
				assert.NoError(t, err)
				assert.Equal(t, "bar", m["foo"])
				tags := m["tags"].([]any)
				assert.Len(t, tags, 1)
			},
		},
		{
			name: "empty",
			pb:   PropertyBag{},
			checkFn: func(t *testing.T, data []byte) {
				assert.Equal(t, "{}", string(data))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.pb.MarshalJSON()
			assert.NoError(t, err)
			tt.checkFn(t, data)
		})
	}
}

func TestPropertyBag_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		checkFn func(t *testing.T, pb *PropertyBag)
	}{
		{
			name: "with additional properties",
			json: `{"key1":"value1","key2":42}`,
			checkFn: func(t *testing.T, pb *PropertyBag) {
				assert.Equal(t, "value1", pb.AdditionalProperties["key1"])
				assert.Equal(t, float64(42), pb.AdditionalProperties["key2"])
			},
		},
		{
			name: "with tags",
			json: `{"tags":["tag1","tag2"]}`,
			checkFn: func(t *testing.T, pb *PropertyBag) {
				assert.Len(t, pb.Tags, 2)
				assert.Equal(t, "tag1", pb.Tags[0])
				assert.Equal(t, "tag2", pb.Tags[1])
			},
		},
		{
			name: "with both",
			json: `{"foo":"bar","tags":["mytag"]}`,
			checkFn: func(t *testing.T, pb *PropertyBag) {
				assert.Equal(t, "bar", pb.AdditionalProperties["foo"])
				assert.Len(t, pb.Tags, 1)
			},
		},
		{
			name: "empty object",
			json: `{}`,
			checkFn: func(t *testing.T, pb *PropertyBag) {
				assert.Empty(t, pb.Tags)
				assert.Empty(t, pb.AdditionalProperties)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pb PropertyBag
			err := pb.UnmarshalJSON([]byte(tt.json))
			assert.NoError(t, err)
			tt.checkFn(t, &pb)
		})
	}
}

func TestPropertyBag_RoundTrip(t *testing.T) {
	original := PropertyBag{
		AdditionalProperties: map[string]any{
			"severity": "high",
			"count":    float64(5),
		},
		Tags: []string{"security", "important"},
	}

	data, err := original.MarshalJSON()
	assert.NoError(t, err)

	var restored PropertyBag
	err = restored.UnmarshalJSON(data)
	assert.NoError(t, err)

	assert.Equal(t, original.Tags, restored.Tags)
	assert.Equal(t, original.AdditionalProperties["severity"], restored.AdditionalProperties["severity"])
	assert.Equal(t, original.AdditionalProperties["count"], restored.AdditionalProperties["count"])
}
