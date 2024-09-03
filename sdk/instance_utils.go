package sdk

import (
	"encoding/json"
	"strings"

	"github.com/featurevisor/featurevisor-go/types"
)

func (i *FeaturevisorInstance) getBucketKeyAndValue(feature *types.Feature, context types.Context) (string, int) {
	bucketKey := i.getBucketKey(feature, context)
	bucketValue := getBucketedNumber(bucketKey)

	if i.configureBucketValue != nil {
		bucketValue = i.configureBucketValue(feature, context, bucketValue)
	}

	return bucketKey, bucketValue
}

func (i *FeaturevisorInstance) getBucketKey(feature *types.Feature, context types.Context) string {
	featureKey := feature.Key
	var attributeKeys []string

	switch bucketBy := feature.BucketBy.(type) {
	case string:
		attributeKeys = []string{bucketBy}
	case []string:
		attributeKeys = bucketBy
	case map[string]interface{}:
		if orKeys, ok := bucketBy["or"].([]string); ok {
			attributeKeys = orKeys
		}
	}

	bucketKey := []string{}

	for _, attributeKey := range attributeKeys {
		if attributeValue, ok := context[types.AttributeKey(attributeKey)]; ok {
			bucketKey = append(bucketKey, toString(attributeValue))
			if len(bucketKey) > 0 && feature.BucketBy.(map[string]interface{})["or"] != nil {
				break
			}
		}
	}

	bucketKey = append(bucketKey, string(featureKey))
	result := joinBucketKey(bucketKey, i.bucketKeySeparator)

	if i.configureBucketKey != nil {
		return i.configureBucketKey(feature, context, result)
	}

	return result
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return string(v)
	case float64:
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(jsonBytes)
	}
}

func joinBucketKey(parts []string, separator string) string {
	return strings.Join(parts, separator)
}

func findVariableSchema(variablesSchema []types.VariableSchema, variableKey types.VariableKey) *types.VariableSchema {
	for _, schema := range variablesSchema {
		if schema.Key == variableKey {
			return &schema
		}
	}
	return nil
}

func parseConditions(conditionsJSON json.RawMessage) types.Condition {
	var conditions types.Condition
	err := json.Unmarshal(conditionsJSON, &conditions)
	if err != nil {
		return types.Condition{}
	}
	return conditions
}

func parseSegments(segmentsJSON json.RawMessage) types.GroupSegment {
	var segments types.GroupSegment
	err := json.Unmarshal(segmentsJSON, &segments)
	if err != nil {
		return nil
	}
	return segments
}

func findForceFromFeature(feature *types.Feature, context types.Context, datafileReader *DatafileReader, logger Logger) (*types.Force, int) {
	for i, force := range feature.Force {
		if force.Conditions != nil {
			conditions := parseConditions(force.Conditions)
			if allConditionsAreMatched(conditions, context, logger) {
				return &force, i
			}
		}
		if force.Segments != nil {
			segments := parseSegments(force.Segments)
			if allGroupSegmentsAreMatched(segments, context, datafileReader, logger) {
				return &force, i
			}
		}
	}
	return nil, -1
}

func getMatchedTraffic(traffic []types.Traffic, context types.Context, datafileReader *DatafileReader, logger Logger) *types.Traffic {
	for _, t := range traffic {
		if t.Segments != nil {
			segments := parseSegments(t.Segments)
			if allGroupSegmentsAreMatched(segments, context, datafileReader, logger) {
				return &t
			}
		}
	}
	return nil
}

func getMatchedTrafficAndAllocation(traffic []types.Traffic, context types.Context, bucketValue int, datafileReader *DatafileReader, logger Logger) (*types.Traffic, *types.Allocation) {
	matchedTraffic := getMatchedTraffic(traffic, context, datafileReader, logger)
	if matchedTraffic == nil {
		return nil, nil
	}

	for _, allocation := range matchedTraffic.Allocation {
		if bucketValue >= int(allocation.Range[0]) && bucketValue < int(allocation.Range[1]) {
			return matchedTraffic, &allocation
		}
	}

	return matchedTraffic, nil
}
