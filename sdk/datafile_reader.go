package sdk

import (
	"encoding/json"

	"github.com/featurevisor/featurevisor-go/types"
)

func parseJSONConditionsIfStringified(record map[string]interface{}, key string) map[string]interface{} {
	if value, ok := record[key].(string); ok && value != "*" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			record[key] = parsed
		}
	}
	return record
}

type DatafileReader struct {
	schemaVersion string
	revision      string
	attributes    []types.Attribute
	segments      []types.Segment
	features      []types.Feature
}

func NewDatafileReader(datafileJSON types.DatafileContent) *DatafileReader {
	return &DatafileReader{
		schemaVersion: datafileJSON.SchemaVersion,
		revision:      datafileJSON.Revision,
		segments:      datafileJSON.Segments,
		attributes:    datafileJSON.Attributes,
		features:      datafileJSON.Features,
	}
}

func (dr *DatafileReader) GetRevision() string {
	return dr.revision
}

func (dr *DatafileReader) GetSchemaVersion() string {
	return dr.schemaVersion
}

func (dr *DatafileReader) GetAllAttributes() []types.Attribute {
	return dr.attributes
}

func (dr *DatafileReader) GetAttribute(attributeKey types.AttributeKey) *types.Attribute {
	for _, a := range dr.attributes {
		if a.Key == attributeKey {
			return &a
		}
	}
	return nil
}

func (dr *DatafileReader) GetSegment(segmentKey types.SegmentKey) *types.Segment {
	for _, s := range dr.segments {
		if s.Key == segmentKey {
			segment := s
			conditionsJSON, _ := json.Marshal(s.Conditions)
			var conditions map[string]interface{}
			json.Unmarshal(conditionsJSON, &conditions)
			parseJSONConditionsIfStringified(conditions, "conditions")
			json.Unmarshal(conditionsJSON, &segment.Conditions)
			return &segment
		}
	}
	return nil
}

func (dr *DatafileReader) GetFeature(featureKey types.FeatureKey) *types.Feature {
	for _, f := range dr.features {
		if f.Key == featureKey {
			return &f
		}
	}
	return nil
}
