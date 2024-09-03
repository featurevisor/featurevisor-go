package types

import "encoding/json"

type Operator string

const (
	OperatorEquals                 Operator = "equals"
	OperatorNotEquals              Operator = "notEquals"
	OperatorGreaterThan            Operator = "greaterThan"
	OperatorGreaterThanOrEquals    Operator = "greaterThanOrEquals"
	OperatorLessThan               Operator = "lessThan"
	OperatorLessThanOrEquals       Operator = "lessThanOrEquals"
	OperatorContains               Operator = "contains"
	OperatorNotContains            Operator = "notContains"
	OperatorStartsWith             Operator = "startsWith"
	OperatorEndsWith               Operator = "endsWith"
	OperatorSemverEquals           Operator = "semverEquals"
	OperatorSemverNotEquals        Operator = "semverNotEquals"
	OperatorSemverGreaterThan      Operator = "semverGreaterThan"
	OperatorSemverGreaterThanOrEquals Operator = "semverGreaterThanOrEquals"
	OperatorSemverLessThan         Operator = "semverLessThan"
	OperatorSemverLessThanOrEquals Operator = "semverLessThanOrEquals"
	OperatorBefore                 Operator = "before"
	OperatorAfter                  Operator = "after"
	OperatorIn                     Operator = "in"
	OperatorNotIn                  Operator = "notIn"
)

type ConditionValue interface{}

type PlainCondition struct {
	Attribute AttributeKey   `json:"attribute"`
	Operator  Operator       `json:"operator"`
	Value     ConditionValue `json:"value"`
}

type AndCondition struct {
	And []Condition `json:"and"`
}

type OrCondition struct {
	Or []Condition `json:"or"`
}

type NotCondition struct {
	Not []Condition `json:"not"`
}

type Condition struct {
	PlainCondition
	AndCondition
	OrCondition
	NotCondition
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.PlainCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.AndCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.OrCondition); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, &c.NotCondition); err == nil {
		return nil
	}
	return json.Unmarshal(data, &c.PlainCondition)
}
