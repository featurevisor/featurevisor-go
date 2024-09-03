package types

type DatafileContent struct {
	SchemaVersion string      `json:"schemaVersion"`
	Revision      string      `json:"revision"`
	Attributes    []Attribute `json:"attributes"`
	Segments      []Segment   `json:"segments"`
	Features      []Feature   `json:"features"`
}
