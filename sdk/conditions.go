package sdk

import (
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/featurevisor/featurevisor-go/types"
)

func conditionIsMatched(condition types.PlainCondition, context types.Context) bool {
	attribute := condition.Attribute
	operator := condition.Operator
	value := condition.Value

	switch operator {
	case types.OperatorEquals:
		return context[attribute] == value
	case types.OperatorNotEquals:
		return context[attribute] != value
	case types.OperatorBefore, types.OperatorAfter:
		valueInContext, ok := context[attribute].(string)
		if !ok {
			return false
		}
		dateInContext, err := time.Parse(time.RFC3339, valueInContext)
		if err != nil {
			return false
		}
		dateInCondition, err := time.Parse(time.RFC3339, value.(string))
		if err != nil {
			return false
		}
		if operator == types.OperatorBefore {
			return dateInContext.Before(dateInCondition)
		}
		return dateInContext.After(dateInCondition)
	case types.OperatorIn, types.OperatorNotIn:
		valueInContext, ok := context[attribute].(string)
		if !ok {
			return false
		}
		valueArray, ok := value.([]interface{})
		if !ok {
			return false
		}
		found := false
		for _, v := range valueArray {
			if v == valueInContext {
				found = true
				break
			}
		}
		if operator == types.OperatorIn {
			return found
		}
		return !found
	case types.OperatorContains, types.OperatorNotContains, types.OperatorStartsWith, types.OperatorEndsWith:
		valueInContext, ok := context[attribute].(string)
		if !ok {
			return false
		}
		valueString, ok := value.(string)
		if !ok {
			return false
		}
		switch operator {
		case types.OperatorContains:
			return strings.Contains(valueInContext, valueString)
		case types.OperatorNotContains:
			return !strings.Contains(valueInContext, valueString)
		case types.OperatorStartsWith:
			return strings.HasPrefix(valueInContext, valueString)
		case types.OperatorEndsWith:
			return strings.HasSuffix(valueInContext, valueString)
		}
	case types.OperatorSemverEquals, types.OperatorSemverNotEquals, types.OperatorSemverGreaterThan,
		types.OperatorSemverGreaterThanOrEquals, types.OperatorSemverLessThan, types.OperatorSemverLessThanOrEquals:
		valueInContext, ok := context[attribute].(string)
		if !ok {
			return false
		}
		valueString, ok := value.(string)
		if !ok {
			return false
		}
		v1, err := semver.Parse(valueInContext)
		if err != nil {
			return false
		}
		v2, err := semver.Parse(valueString)
		if err != nil {
			return false
		}
		switch operator {
		case types.OperatorSemverEquals:
			return v1.EQ(v2)
		case types.OperatorSemverNotEquals:
			return !v1.EQ(v2)
		case types.OperatorSemverGreaterThan:
			return v1.GT(v2)
		case types.OperatorSemverGreaterThanOrEquals:
			return v1.GTE(v2)
		case types.OperatorSemverLessThan:
			return v1.LT(v2)
		case types.OperatorSemverLessThanOrEquals:
			return v1.LTE(v2)
		}
	case types.OperatorGreaterThan, types.OperatorGreaterThanOrEquals, types.OperatorLessThan, types.OperatorLessThanOrEquals:
		valueInContext, ok := context[attribute].(float64)
		if !ok {
			return false
		}
		valueFloat, ok := value.(float64)
		if !ok {
			return false
		}
		switch operator {
		case types.OperatorGreaterThan:
			return valueInContext > valueFloat
		case types.OperatorGreaterThanOrEquals:
			return valueInContext >= valueFloat
		case types.OperatorLessThan:
			return valueInContext < valueFloat
		case types.OperatorLessThanOrEquals:
			return valueInContext <= valueFloat
		}
	}

	return false
}

func allConditionsAreMatched(conditions interface{}, context types.Context, logger Logger) bool {
	switch c := conditions.(type) {
	case types.PlainCondition:
		return conditionIsMatched(c, context)
	case types.AndCondition:
		for _, condition := range c.And {
			if !allConditionsAreMatched(condition, context, logger) {
				return false
			}
		}
		return true
	case types.OrCondition:
		for _, condition := range c.Or {
			if allConditionsAreMatched(condition, context, logger) {
				return true
			}
		}
		return false
	case types.NotCondition:
		return !allConditionsAreMatched(types.AndCondition{And: c.Not}, context, logger)
	case []types.Condition:
		for _, condition := range c {
			if !allConditionsAreMatched(condition, context, logger) {
				return false
			}
		}
		return true
	}

	return false
}
