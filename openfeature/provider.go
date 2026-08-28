// Package openfeature provides the OpenFeature adapter for Featurevisor.
package openfeature

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go/v3"
	of "github.com/open-feature/go-sdk/openfeature"
)

type TrackingEvent struct {
	Name    string
	Context of.EvaluationContext
	Details of.TrackingEventDetails
}

type Options struct {
	Featurevisor         *featurevisor.Featurevisor
	FeaturevisorOptions  featurevisor.FeaturevisorOptions
	TargetingKeyField    string
	KeySeparator         string
	VariationKey         string
	GlobalVariablePrefix string
	OnTrack              func(TrackingEvent)
}

type Provider struct {
	featurevisor         *featurevisor.Featurevisor
	targetingKeyField    string
	keySeparator         string
	variationKey         string
	globalVariablePrefix string
	onTrack              func(TrackingEvent)
	datafileError        string
	datafileUnsubscribe  featurevisor.Unsubscribe
	ownsFeaturevisor     bool
}

func NewProvider(options Options) *Provider {
	p := &Provider{
		targetingKeyField:    valueOr(options.TargetingKeyField, "userId"),
		keySeparator:         valueOr(options.KeySeparator, ":"),
		variationKey:         valueOr(options.VariationKey, "variation"),
		globalVariablePrefix: valueOr(options.GlobalVariablePrefix, "variable"),
		onTrack:              options.OnTrack,
	}
	if strings.Contains(p.globalVariablePrefix, p.keySeparator) {
		panic("globalVariablePrefix cannot contain keySeparator")
	}
	if options.Featurevisor != nil {
		p.featurevisor = options.Featurevisor
	} else {
		p.ownsFeaturevisor = true
		fvOptions := options.FeaturevisorOptions
		if datafile, ok := fvOptions.Datafile.(string); ok && datafile != "" && !json.Valid([]byte(datafile)) {
			p.datafileError = "Could not parse datafile"
		}
		originalHandler := fvOptions.OnDiagnostic
		fvOptions.OnDiagnostic = func(diagnostic featurevisor.FeaturevisorDiagnostic) {
			if diagnostic.Code == "invalid_datafile" {
				p.datafileError = diagnostic.Message
			}
			if diagnostic.Code == "datafile_set" {
				p.datafileError = ""
			}
			if originalHandler != nil {
				originalHandler(diagnostic)
			}
		}
		p.featurevisor = featurevisor.CreateFeaturevisor(fvOptions)
	}
	p.datafileUnsubscribe = p.featurevisor.On(featurevisor.EventNameDatafileSet, func(featurevisor.EventDetails) {
		p.datafileError = ""
	})
	return p
}

func (p *Provider) Featurevisor() *featurevisor.Featurevisor { return p.featurevisor }
func (p *Provider) Metadata() of.Metadata                    { return of.Metadata{Name: "Featurevisor"} }
func (p *Provider) Hooks() []of.Hook                         { return nil }
func (p *Provider) Init(of.EvaluationContext) error          { return nil }
func (p *Provider) Shutdown() {
	if p.datafileUnsubscribe != nil {
		p.datafileUnsubscribe()
	}
	if p.ownsFeaturevisor {
		p.featurevisor.Close()
	}
}

func (p *Provider) Track(_ context.Context, name string, evaluationContext of.EvaluationContext, details of.TrackingEventDetails) {
	if p.onTrack != nil {
		p.onTrack(TrackingEvent{Name: name, Context: evaluationContext, Details: details})
	}
}

func (p *Provider) BooleanEvaluation(_ context.Context, flag string, defaultValue bool, flatCtx of.FlattenedContext) of.BoolResolutionDetail {
	value, detail := p.resolve(flag, defaultValue, flatCtx, "boolean")
	resolved, ok := value.(bool)
	if !ok {
		resolved = defaultValue
	}
	return of.BoolResolutionDetail{Value: resolved, ProviderResolutionDetail: detail}
}

func (p *Provider) StringEvaluation(_ context.Context, flag string, defaultValue string, flatCtx of.FlattenedContext) of.StringResolutionDetail {
	value, detail := p.resolve(flag, defaultValue, flatCtx, "string")
	resolved, ok := value.(string)
	if !ok {
		resolved = defaultValue
	}
	return of.StringResolutionDetail{Value: resolved, ProviderResolutionDetail: detail}
}

func (p *Provider) FloatEvaluation(_ context.Context, flag string, defaultValue float64, flatCtx of.FlattenedContext) of.FloatResolutionDetail {
	value, detail := p.resolve(flag, defaultValue, flatCtx, "number")
	resolved, ok := asFloat(value)
	if !ok {
		resolved = defaultValue
	}
	return of.FloatResolutionDetail{Value: resolved, ProviderResolutionDetail: detail}
}

func (p *Provider) IntEvaluation(_ context.Context, flag string, defaultValue int64, flatCtx of.FlattenedContext) of.IntResolutionDetail {
	value, detail := p.resolve(flag, defaultValue, flatCtx, "number")
	resolved, ok := asInt(value)
	if !ok {
		resolved = defaultValue
	}
	return of.IntResolutionDetail{Value: resolved, ProviderResolutionDetail: detail}
}

func (p *Provider) ObjectEvaluation(_ context.Context, flag string, defaultValue any, flatCtx of.FlattenedContext) of.InterfaceResolutionDetail {
	value, detail := p.resolve(flag, defaultValue, flatCtx, "object")
	if !isObject(value) {
		value = defaultValue
	}
	return of.InterfaceResolutionDetail{Value: value, ProviderResolutionDetail: detail}
}

func (p *Provider) resolve(flag string, defaultValue any, flatCtx of.FlattenedContext, expected string) (any, of.ProviderResolutionDetail) {
	if p.datafileError != "" {
		return defaultValue, errorDetail(of.NewParseErrorResolutionError(p.datafileError))
	}
	featureKey, selector := splitKey(flag, p.keySeparator)
	context := normalizeContext(flatCtx)
	if targetingKey, ok := flatCtx[of.TargetingKey].(string); ok && targetingKey != "" {
		context[p.targetingKeyField] = targetingKey
	}

	var evaluation featurevisor.Evaluation
	var value any
	if featureKey == p.globalVariablePrefix && selector != "" {
		evaluation = p.featurevisor.EvaluateGlobalVariable(selector, context, featurevisor.OverrideOptions{})
		value = evaluation.VariableValue
		if evaluation.GlobalVariable != nil && evaluation.GlobalVariable.Type == featurevisor.VariableTypeJSON {
			if raw, ok := value.(string); ok {
				var parsed any
				if json.Unmarshal([]byte(raw), &parsed) == nil {
					value = parsed
				}
			}
		}
	} else if selector == "" {
		if expected != "boolean" {
			return defaultValue, typeMismatch(flag, expected)
		}
		evaluation = p.featurevisor.EvaluateFlag(featureKey, context, featurevisor.OverrideOptions{})
		if evaluation.Enabled != nil {
			value = *evaluation.Enabled
		}
	} else if selector == p.variationKey {
		evaluation = p.featurevisor.EvaluateVariation(featureKey, context, featurevisor.OverrideOptions{})
		if evaluation.VariationValue != nil {
			value = string(*evaluation.VariationValue)
		} else if evaluation.Variation != nil {
			value = string(evaluation.Variation.Value)
		}
	} else {
		evaluation = p.featurevisor.EvaluateVariable(featureKey, selector, context, featurevisor.OverrideOptions{})
		value = evaluation.VariableValue
		if evaluation.VariableSchema != nil && evaluation.VariableSchema.Type == featurevisor.VariableTypeJSON {
			if raw, ok := value.(string); ok {
				var parsed any
				if json.Unmarshal([]byte(raw), &parsed) == nil {
					value = parsed
				}
			}
		}
	}

	detail := detailFor(evaluation, p.featurevisor)
	if detail.Error() != nil {
		return defaultValue, detail
	}
	if value == nil {
		return defaultValue, detail
	}
	if !matches(value, expected) {
		return defaultValue, typeMismatchWithMetadata(flag, expected, detail.FlagMetadata)
	}
	return value, detail
}

func detailFor(e featurevisor.Evaluation, fv *featurevisor.Featurevisor) of.ProviderResolutionDetail {
	metadata := of.FlagMetadata{"featurevisorReason": string(e.Reason), "schemaVersion": fv.GetSchemaVersion()}
	if e.FeatureKey != "" {
		metadata["featureKey"] = string(e.FeatureKey)
	}
	if revision := fv.GetRevision(); revision != "" {
		metadata["revision"] = revision
	}
	if e.VariableKey != nil {
		metadata["variableKey"] = string(*e.VariableKey)
	}
	if e.RuleKey != nil {
		metadata["ruleKey"] = string(*e.RuleKey)
	}
	if e.BucketKey != nil {
		metadata["bucketKey"] = string(*e.BucketKey)
	}
	if e.BucketValue != nil {
		metadata["bucketValue"] = int(*e.BucketValue)
	}
	if e.ForceIndex != nil {
		metadata["forceIndex"] = *e.ForceIndex
	}
	if e.VariableOverrideIndex != nil {
		metadata["variableOverrideIndex"] = *e.VariableOverrideIndex
	}
	if e.VariableOverrideKey != nil {
		metadata["variableOverrideKey"] = *e.VariableOverrideKey
	}
	detail := of.ProviderResolutionDetail{Reason: reasonFor(e.Reason), FlagMetadata: metadata}
	if e.VariationValue != nil {
		detail.Variant = string(*e.VariationValue)
	} else if e.Variation != nil {
		detail.Variant = string(e.Variation.Value)
	}
	switch e.Reason {
	case featurevisor.EvaluationReasonFeatureNotFound:
		detail.ResolutionError = of.NewFlagNotFoundResolutionError(fmt.Sprintf("Feature %q was not found", e.FeatureKey))
	case featurevisor.EvaluationReasonVariableNotFound:
		if e.FeatureKey == "" {
			detail.ResolutionError = of.NewFlagNotFoundResolutionError(fmt.Sprintf("Variable %q was not found", valueOrPointer(e.VariableKey)))
		} else {
			detail.ResolutionError = of.NewFlagNotFoundResolutionError(fmt.Sprintf("Variable %q was not found for feature %q", valueOrPointer(e.VariableKey), e.FeatureKey))
		}
	case featurevisor.EvaluationReasonNoVariations:
		detail.ResolutionError = of.NewFlagNotFoundResolutionError(fmt.Sprintf("Feature %q has no variations", e.FeatureKey))
	case featurevisor.EvaluationReasonError:
		detail.ResolutionError = of.NewGeneralResolutionError("Featurevisor evaluation failed", e.Error)
	}
	return detail
}

func reasonFor(reason featurevisor.EvaluationReason) of.Reason {
	switch reason {
	case featurevisor.EvaluationReasonFeatureNotFound, featurevisor.EvaluationReasonVariableNotFound, featurevisor.EvaluationReasonNoVariations, featurevisor.EvaluationReasonError:
		return of.ErrorReason
	case featurevisor.EvaluationReasonRequired, featurevisor.EvaluationReasonForced, featurevisor.EvaluationReasonSticky, featurevisor.EvaluationReasonRule, featurevisor.EvaluationReasonVariableOverrideVariation, featurevisor.EvaluationReasonVariableOverrideRule:
		return of.TargetingMatchReason
	case featurevisor.EvaluationReasonAllocated:
		return of.SplitReason
	case featurevisor.EvaluationReasonDisabled, featurevisor.EvaluationReasonVariationDisabled, featurevisor.EvaluationReasonVariableDisabled:
		return of.DisabledReason
	case featurevisor.EvaluationReasonRequiredFeaturesUnmet:
		return of.DisabledReason
	default:
		return of.DefaultReason
	}
}

func errorDetail(err of.ResolutionError) of.ProviderResolutionDetail {
	return of.ProviderResolutionDetail{Reason: of.ErrorReason, ResolutionError: err}
}
func typeMismatch(flag, expected string) of.ProviderResolutionDetail {
	return errorDetail(of.NewTypeMismatchResolutionError(fmt.Sprintf("Flag %q did not resolve to a %s value", flag, expected)))
}
func typeMismatchWithMetadata(flag, expected string, metadata of.FlagMetadata) of.ProviderResolutionDetail {
	d := typeMismatch(flag, expected)
	d.FlagMetadata = metadata
	return d
}
func splitKey(key, separator string) (string, string) {
	if i := strings.Index(key, separator); i >= 0 {
		return key[:i], key[i+len(separator):]
	}
	return key, ""
}
func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func valueOrPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeContext(input map[string]any) featurevisor.Context {
	result := featurevisor.Context{}
	for key, value := range input {
		result[key] = normalizeValue(value)
	}
	return result
}
func normalizeValue(value any) any {
	if date, ok := value.(time.Time); ok {
		return date.Format(time.RFC3339Nano)
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = normalizeValue(rv.Index(i).Interface())
		}
		return result
	}
	if values, ok := value.(map[string]any); ok {
		return normalizeContext(values)
	}
	return value
}
func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	}
	return 0, false
}
func asInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), math.Trunc(v) == v && !math.IsInf(v, 0) && !math.IsNaN(v)
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}
func isObject(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array || kind == reflect.Struct
}
func matches(value any, expected string) bool {
	switch expected {
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := asFloat(value)
		return ok
	case "object":
		return isObject(value)
	}
	return false
}
