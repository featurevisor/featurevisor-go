package types

type AttributeKey string
type AttributeValue interface{}

type Context map[AttributeKey]AttributeValue

type AttributeType string

const (
	AttributeTypeBoolean AttributeType = "boolean"
	AttributeTypeString  AttributeType = "string"
	AttributeTypeInteger AttributeType = "integer"
	AttributeTypeDouble  AttributeType = "double"
	AttributeTypeDate    AttributeType = "date"
	AttributeTypeSemver  AttributeType = "semver"
)

type Attribute struct {
	Key     AttributeKey `json:"key"`
	Type    AttributeType `json:"type"`
	Capture *bool         `json:"capture,omitempty"`
}
