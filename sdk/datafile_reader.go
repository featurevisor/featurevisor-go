package sdk

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// DatafileReaderOptions contains options for creating a datafile reader
type DatafileReaderOptions struct {
	Datafile DatafileContent
	Logger   *Logger
}

// ForceResult represents the result of a force lookup
type ForceResult struct {
	Force      *Force `json:"force,omitempty"`
	ForceIndex *int   `json:"forceIndex,omitempty"`
}

// DatafileReader provides functionality to read and query datafile content
type DatafileReader struct {
	schemaVersion string
	revision      string
	segments      map[SegmentKey]Segment
	features      map[FeatureKey]Feature
	logger        *Logger
	regexCache    map[string]*regexp.Regexp
}

// NewDatafileReader creates a new datafile reader instance
func NewDatafileReader(options DatafileReaderOptions) *DatafileReader {
	return &DatafileReader{
		schemaVersion: options.Datafile.SchemaVersion,
		revision:      options.Datafile.Revision,
		segments:      options.Datafile.Segments,
		features:      options.Datafile.Features,
		logger:        options.Logger,
		regexCache:    make(map[string]*regexp.Regexp),
	}
}

// GetRevision returns the revision of the datafile
func (d *DatafileReader) GetRevision() string {
	return d.revision
}

// GetSchemaVersion returns the schema version of the datafile
func (d *DatafileReader) GetSchemaVersion() string {
	return d.schemaVersion
}

// GetSegment returns a segment by its key
func (d *DatafileReader) GetSegment(segmentKey SegmentKey) *Segment {
	segment, exists := d.segments[segmentKey]

	if !exists {
		return nil
	}

	segment.Conditions = d.parseConditionsIfStringified(segment.Conditions)

	return &segment
}

// GetFeatureKeys returns all feature keys
func (d *DatafileReader) GetFeatureKeys() []string {
	keys := make([]string, 0, len(d.features))
	for key := range d.features {
		keys = append(keys, string(key))
	}
	return keys
}

// GetFeature returns a feature by its key
func (d *DatafileReader) GetFeature(featureKey FeatureKey) *Feature {
	feature, exists := d.features[featureKey]
	if !exists {
		return nil
	}

	// Parse required features if they exist
	if feature.Required != nil {
		feature.Required = d.parseRequiredIfStringified(feature.Required)
	}

	return &feature
}

// GetVariableKeys returns the variable keys for a feature
func (d *DatafileReader) GetVariableKeys(featureKey FeatureKey) []string {
	feature := d.GetFeature(featureKey)

	if feature == nil || feature.VariablesSchema == nil {
		return []string{}
	}

	keys := make([]string, 0, len(feature.VariablesSchema))
	for key := range feature.VariablesSchema {
		keys = append(keys, string(key))
	}
	return keys
}

// HasVariations checks if a feature has variations
func (d *DatafileReader) HasVariations(featureKey FeatureKey) bool {
	feature := d.GetFeature(featureKey)

	if feature == nil {
		return false
	}

	return feature.Variations != nil && len(feature.Variations) > 0
}

// GetRegex returns a regex pattern with caching
func (d *DatafileReader) GetRegex(regexString string, regexFlags string) *regexp.Regexp {
	flags := regexFlags
	if flags == "" {
		flags = ""
	}

	cacheKey := fmt.Sprintf("%s-%s", regexString, flags)

	if d.regexCache[cacheKey] != nil {
		return d.regexCache[cacheKey]
	}

	regex := regexp.MustCompile(regexString)
	d.regexCache[cacheKey] = regex

	return regex
}

// AllConditionsAreMatched checks if all conditions are matched given a context
func (d *DatafileReader) AllConditionsAreMatched(conditions Condition, context Context) bool {
	// Add error handling wrapper like in TypeScript version
	defer func() {
		if r := recover(); r != nil {
			d.logger.Warn("Error in condition matching", LogDetails{
				"error":      r,
				"conditions": conditions,
				"context":    context,
			})
		}
	}()

	// Handle string conditions
	if conditionStr, ok := conditions.(string); ok {
		if conditionStr == "*" {
			return true
		}
		return false
	}

	// Handle plain conditions
	if plainCondition, ok := conditions.(PlainCondition); ok {
		getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
			return d.GetRegex(regexString, regexFlags)
		}

		matched := ConditionIsMatched(plainCondition, context, getRegex)
		return matched
	}

	// Handle conditions from JSON unmarshaling (map[string]interface{})
	if conditionMap, ok := conditions.(map[string]interface{}); ok {

		// Check if it's a plain condition
		if attribute, ok := conditionMap["attribute"].(string); ok {
			if operator, ok := conditionMap["operator"].(string); ok {
				// Handle operators that don't have a value (exists, notExists)
				if operator == "exists" || operator == "notExists" {
					plainCondition := PlainCondition{
						Attribute: AttributeKey(attribute),
						Operator:  Operator(operator),
						Value:     nil, // exists/notExists don't have values
					}
					getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
						return d.GetRegex(regexString, regexFlags)
					}

					matched := ConditionIsMatched(plainCondition, context, getRegex)
					return matched
				}

				// Handle operators that have a value
				if value, ok := conditionMap["value"]; ok {
					conditionValue := ConditionValue(value)
					plainCondition := PlainCondition{
						Attribute: AttributeKey(attribute),
						Operator:  Operator(operator),
						Value:     &conditionValue,
					}
					getRegex := func(regexString string, regexFlags string) *regexp.Regexp {
						return d.GetRegex(regexString, regexFlags)
					}

					// Add error handling like in TypeScript version
					matched := ConditionIsMatched(plainCondition, context, getRegex)
					return matched
				}
			}
		}
		// Check if it's an and condition
		if andConditions, ok := conditionMap["and"].([]interface{}); ok {
			for _, condition := range andConditions {
				if !d.AllConditionsAreMatched(condition, context) {
					return false
				}
			}
			return true
		}
		// Check if it's an or condition
		if orConditions, ok := conditionMap["or"].([]interface{}); ok {
			for _, condition := range orConditions {
				if d.AllConditionsAreMatched(condition, context) {
					return true
				}
			}
			return false
		}
		// Check if it's a not condition
		if notCondition, ok := conditionMap["not"]; ok {
			return !d.AllConditionsAreMatched(notCondition, context)
		}
	}

	// Handle and conditions
	if andCondition, ok := conditions.(AndCondition); ok {
		for _, condition := range andCondition.And {
			if !d.AllConditionsAreMatched(condition, context) {
				return false
			}
		}
		return true
	}

	// Handle or conditions
	if orCondition, ok := conditions.(OrCondition); ok {
		for _, condition := range orCondition.Or {
			if d.AllConditionsAreMatched(condition, context) {
				return true
			}
		}
		return false
	}

	// Handle not conditions
	if notCondition, ok := conditions.(NotCondition); ok {
		return !d.AllConditionsAreMatched(notCondition.Not, context)
	}

	// Handle array of conditions (from JSON unmarshaling)
	if conditionArray, ok := conditions.([]interface{}); ok {
		for _, condition := range conditionArray {
			if !d.AllConditionsAreMatched(condition, context) {
				return false
			}
		}
		return true
	}

	// Handle array of conditions (typed)
	if conditionArray, ok := conditions.([]Condition); ok {
		for _, condition := range conditionArray {
			if !d.AllConditionsAreMatched(condition, context) {
				return false
			}
		}
		return true
	}

	return false
}

// SegmentIsMatched checks if a segment is matched given a context
func (d *DatafileReader) SegmentIsMatched(segment *Segment, context Context) bool {
	return d.AllConditionsAreMatched(segment.Conditions, context)
}

// AllSegmentsAreMatched checks if all segments are matched given a context
func (d *DatafileReader) AllSegmentsAreMatched(groupSegments interface{}, context Context) bool {
	// Add error handling wrapper like in TypeScript version
	defer func() {
		if r := recover(); r != nil {
			d.logger.Warn("Error in segment matching", LogDetails{
				"error":         r,
				"groupSegments": groupSegments,
				"context":       context,
			})
		}
	}()
	// Handle wildcard
	if groupSegments == "*" {
		d.logger.Debug("matched wildcard segment", LogDetails{
			"segments": groupSegments,
		})
		return true
	}

	// Handle string segment
	if segmentKey, ok := groupSegments.(string); ok {
		segment := d.GetSegment(SegmentKey(segmentKey))
		if segment != nil {
			matched := d.SegmentIsMatched(segment, context)
			d.logger.Debug("checked single segment", LogDetails{
				"segment": segmentKey,
				"matched": matched,
			})
			return matched
		}
		return false
	}

	// Handle and segments
	if andSegments, ok := groupSegments.(AndGroupSegment); ok {
		for _, groupSegment := range andSegments.And {
			if !d.AllSegmentsAreMatched(groupSegment, context) {
				return false
			}
		}
		return true
	}

	// Handle or segments
	if orSegments, ok := groupSegments.(OrGroupSegment); ok {
		for _, groupSegment := range orSegments.Or {
			if d.AllSegmentsAreMatched(groupSegment, context) {
				return true
			}
		}
		return false
	}

	// Handle not segments
	if notSegments, ok := groupSegments.(NotGroupSegment); ok {
		if d.AllSegmentsAreMatched(notSegments.Not, context) {
			return false
		}
		return true
	}

	// Handle array of segments (from JSON unmarshaling)
	if segmentArray, ok := groupSegments.([]interface{}); ok {
		for _, groupSegment := range segmentArray {
			if !d.AllSegmentsAreMatched(groupSegment, context) {
				return false
			}
		}
		return true
	}

	// Handle array of segments (typed)
	if segmentArray, ok := groupSegments.([]GroupSegment); ok {
		for _, groupSegment := range segmentArray {
			if !d.AllSegmentsAreMatched(groupSegment, context) {
				return false
			}
		}
		return true
	}

	// Handle segments from JSON unmarshaling (map[string]interface{})
	if segmentMap, ok := groupSegments.(map[string]interface{}); ok {
		// Check if it's an "or" segment
		if orSegments, ok := segmentMap["or"].([]interface{}); ok {
			for _, segment := range orSegments {
				if d.AllSegmentsAreMatched(segment, context) {
					return true
				}
			}
			return false
		}

		// Check if it's an "and" segment
		if andSegments, ok := segmentMap["and"].([]interface{}); ok {
			for _, segment := range andSegments {
				if !d.AllSegmentsAreMatched(segment, context) {
					return false
				}
			}
			return true
		}

		// Check if it's a "not" segment
		if notSegment, ok := segmentMap["not"]; ok {
			return !d.AllSegmentsAreMatched(notSegment, context)
		}
	}

	d.logger.Debug("no segments matched", LogDetails{
		"segments": groupSegments,
	})
	return false
}

// GetMatchedTraffic returns the matched traffic for a given context
func (d *DatafileReader) GetMatchedTraffic(traffic []Traffic, context Context) *Traffic {
	for _, t := range traffic {
		segments := d.parseSegmentsIfStringified(t.Segments)
		if d.AllSegmentsAreMatched(segments, context) {
			d.logger.Debug("matched traffic rule", LogDetails{
				"ruleKey":  t.Key,
				"segments": t.Segments,
			})
			return &t
		}
	}
	return nil
}

// GetMatchedAllocation returns the matched allocation for a given bucket value
func (d *DatafileReader) GetMatchedAllocation(traffic *Traffic, bucketValue int) *Allocation {
	if traffic.Allocation == nil {
		return nil
	}

	for _, allocation := range traffic.Allocation {
		start := allocation.Range[0]
		end := allocation.Range[1]

		if start <= bucketValue && end >= bucketValue {
			return &allocation
		}
	}

	return nil
}

// GetMatchedForce returns the matched force for a given feature and context
func (d *DatafileReader) GetMatchedForce(featureKey interface{}, context Context) ForceResult {
	result := ForceResult{
		Force:      nil,
		ForceIndex: nil,
	}

	var feature *Feature

	switch key := featureKey.(type) {
	case FeatureKey:
		feature = d.GetFeature(key)
	case *Feature:
		feature = key
	default:
		return result
	}

	if feature == nil || feature.Force == nil {
		return result
	}

	for i, currentForce := range feature.Force {
		// Check conditions
		if currentForce.Conditions != nil {
			conditions := d.parseConditionsIfStringified(currentForce.Conditions)
			if d.AllConditionsAreMatched(conditions, context) {
				result.Force = &currentForce
				result.ForceIndex = &i
				break
			}
		}

		// Check segments
		if currentForce.Segments != nil {
			segments := d.parseSegmentsIfStringified(currentForce.Segments)
			if d.AllSegmentsAreMatched(segments, context) {
				result.Force = &currentForce
				result.ForceIndex = &i
				break
			}
		}
	}

	return result
}

// parseConditionsIfStringified parses conditions if they are stringified
func (d *DatafileReader) parseConditionsIfStringified(conditions Condition) Condition {
	if conditionStr, ok := conditions.(string); ok {
		if conditionStr == "*" {
			return conditions
		}

		var parsedCondition Condition
		err := json.Unmarshal([]byte(conditionStr), &parsedCondition)
		if err != nil {
			d.logger.Error("Error parsing conditions", LogDetails{
				"error":      err,
				"conditions": conditionStr,
			})
			return conditions
		}

		return parsedCondition
	}

	return conditions
}

// parseSegmentsIfStringified parses segments if they are stringified
func (d *DatafileReader) parseSegmentsIfStringified(segments interface{}) interface{} {
	if segmentStr, ok := segments.(string); ok {
		if strings.HasPrefix(segmentStr, "{") || strings.HasPrefix(segmentStr, "[") {
			var parsedSegments interface{}
			err := json.Unmarshal([]byte(segmentStr), &parsedSegments)
			if err != nil {
				d.logger.Error("Error parsing segments", LogDetails{
					"error":    err,
					"segments": segmentStr,
				})
				return segments
			}
			return parsedSegments
		}
	}

	return segments
}

// parseRequiredIfStringified parses required features if they are stringified
func (d *DatafileReader) parseRequiredIfStringified(required []Required) []Required {
	parsedRequired := make([]Required, len(required))

	for i, req := range required {
		// If it's already a string, it's a simple required feature
		if reqStr, ok := req.(string); ok {
			parsedRequired[i] = reqStr
			continue
		}

		// If it's a map, try to parse it as RequiredWithVariation
		if reqMap, ok := req.(map[string]interface{}); ok {
			if key, exists := reqMap["key"]; exists {
				if variation, exists := reqMap["variation"]; exists {
					// Convert to RequiredWithVariation
					parsedRequired[i] = RequiredWithVariation{
						Key:       FeatureKey(key.(string)),
						Variation: VariationValue(variation.(string)),
					}
					continue
				}
			}
		}

		// If it's already a RequiredWithVariation, keep it as is
		if _, ok := req.(RequiredWithVariation); ok {
			parsedRequired[i] = req
			continue
		}

		// If we can't parse it, keep the original
		parsedRequired[i] = req
	}

	return parsedRequired
}
