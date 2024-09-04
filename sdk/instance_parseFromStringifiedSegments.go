package sdk

import (
	"encoding/json"
	"strings"
)

func (f *FeaturevisorInstance) parseFromStringifiedSegments(value interface{}) interface{} {
	if strValue, ok := value.(string); ok {
		if strings.HasPrefix(strValue, "{") || strings.HasPrefix(strValue, "[") {
			var parsed interface{}
			if err := json.Unmarshal([]byte(strValue), &parsed); err == nil {
				return parsed
			}
		}
	}
	return value
}
