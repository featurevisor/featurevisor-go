package featurevisor

import (
	"fmt"
	"sort"
)

// evaluateParams contains parameters for evaluation
type evaluateParams struct {
	Type           EvaluationType
	FeatureKey     FeatureKey
	VariableKey    *VariableKey
	GlobalVariable bool
}

// evaluateDependencies contains dependencies for evaluation
type evaluateDependencies struct {
	Context                        Context
	diagnosticReporter             *diagnosticReporter
	modulesManager                 *modulesManager
	instanceEvaluationDataProvider *instanceEvaluationDataProvider

	// Instance-internal sticky state. Consumers configure it on an instance.
	sticky          *StickyFeatures
	stickyVariables *StickyVariables

	DefaultVariationValue   *VariationValue
	DefaultVariableValue    VariableValue
	DefaultVariableValueSet bool
}

// EvaluateOptions contains all options for evaluation
type EvaluateOptions struct {
	evaluateParams
	evaluateDependencies
}

// evaluateWithModules evaluates a feature with modules.
func evaluateWithModules(opts EvaluateOptions) Evaluation {
	var evaluation Evaluation

	defer func() {
		if r := recover(); r != nil {
			opts.diagnosticReporter.Error("panic during evaluation", logDetails{
				"error": r,
			})

			// Return error evaluation when panic occurs (matching TypeScript behavior)
			evaluation = Evaluation{
				Type:        opts.Type,
				FeatureKey:  opts.FeatureKey,
				VariableKey: opts.VariableKey,
				Reason:      EvaluationReasonError,
				Error:       fmt.Errorf("panic: %v", r),
			}
		}
	}()

	modulesManager := opts.modulesManager
	modules := modulesManager.GetAll()

	// Run the legacy feature callback and the unified evaluation callback.
	options := opts
	for _, module := range modules {
		if !options.GlobalVariable && module.Before != nil {
			options = module.Before(options)
		}
	}
	for _, module := range modules {
		if module.BeforeEvaluation != nil {
			options = module.BeforeEvaluation(options)
		}
	}

	// evaluate
	if options.GlobalVariable {
		evaluation = evaluateGlobalVariable(options)
	} else {
		evaluation = evaluate(options)
	}

	// default: variation
	if options.DefaultVariationValue != nil &&
		evaluation.Type == EvaluationTypeVariation &&
		evaluation.VariationValue == nil {
		evaluation.VariationValue = options.DefaultVariationValue
	}

	// default: variable
	if options.DefaultVariableValueSet &&
		evaluation.Type == EvaluationTypeVariable &&
		!evaluation.variableValueSet && evaluation.VariableValue == nil {
		evaluation.VariableValue = options.DefaultVariableValue
		evaluation.variableValueSet = true
	}

	// run after modules
	for _, module := range modules {
		if module.AfterEvaluation != nil {
			evaluation = module.AfterEvaluation(evaluation, options)
		}
	}
	for _, module := range modules {
		if !options.GlobalVariable && module.After != nil {
			evaluation = module.After(evaluation, options)
		}
	}

	return evaluation
}

func cleanRequiredFeatureDependencies(dependencies evaluateDependencies) evaluateDependencies {
	dependencies.DefaultVariationValue = nil
	dependencies.DefaultVariableValue = nil
	dependencies.DefaultVariableValueSet = false
	return dependencies
}

func requiredFeatureParts(required Required) (FeatureKey, bool, *VariationValue, bool) {
	switch value := required.(type) {
	case string:
		return FeatureKey(value), true, nil, true
	case RequiredFeature:
		expected := true
		if value.Enabled != nil {
			expected = *value.Enabled
		}
		return value.Feature, expected, value.Variation, value.Feature != ""
	case RequiredWithVariation:
		variation := value.Variation
		return value.Key, true, &variation, value.Key != ""
	case map[string]interface{}:
		if feature, ok := value["feature"].(string); ok {
			expected := true
			if enabled, ok := value["enabled"].(bool); ok {
				expected = enabled
			}
			var variation *VariationValue
			if raw, ok := value["variation"].(string); ok {
				parsed := VariationValue(raw)
				variation = &parsed
			}
			return FeatureKey(feature), expected, variation, feature != ""
		}
		if key, ok := value["key"].(string); ok {
			var variation *VariationValue
			if raw, ok := value["variation"].(string); ok {
				parsed := VariationValue(raw)
				variation = &parsed
			}
			return FeatureKey(key), true, variation, key != ""
		}
	}
	return "", true, nil, false
}

func requiredFeaturesAreMatched(requiredFeatures []Required, dependencies evaluateDependencies) bool {
	dependencies = cleanRequiredFeatureDependencies(dependencies)
	for _, required := range requiredFeatures {
		featureKey, expectedEnabled, expectedVariation, ok := requiredFeatureParts(required)
		if !ok {
			return false
		}
		flag := evaluateWithModules(EvaluateOptions{
			evaluateParams:       evaluateParams{Type: EvaluationTypeFlag, FeatureKey: featureKey},
			evaluateDependencies: dependencies,
		})
		if (flag.Enabled != nil && *flag.Enabled) != expectedEnabled {
			return false
		}
		if expectedVariation != nil {
			variation := evaluateWithModules(EvaluateOptions{
				evaluateParams:       evaluateParams{Type: EvaluationTypeVariation, FeatureKey: featureKey},
				evaluateDependencies: dependencies,
			})
			var actual *VariationValue
			if variation.VariationValue != nil {
				actual = variation.VariationValue
			} else if variation.Variation != nil {
				value := variation.Variation.Value
				actual = &value
			}
			if actual == nil || *actual != *expectedVariation {
				return false
			}
		}
	}
	return true
}

func variableOverrideIsMatched(override VariableOverride, dependencies evaluateDependencies) bool {
	matchedSelector := false
	if override.Conditions != nil {
		matchedSelector = dependencies.instanceEvaluationDataProvider.AllConditionsAreMatched(
			dependencies.instanceEvaluationDataProvider.parseConditionsIfStringified(override.Conditions), dependencies.Context,
		)
	}
	if override.Segments != nil {
		segmentMatched := dependencies.instanceEvaluationDataProvider.AllSegmentsAreMatched(
			dependencies.instanceEvaluationDataProvider.parseSegmentsIfStringified(override.Segments), dependencies.Context,
		)
		if override.Conditions == nil {
			matchedSelector = segmentMatched
		} else {
			matchedSelector = matchedSelector && segmentMatched
		}
	}
	if len(override.RequiredFeatures) > 0 {
		requiredMatched := requiredFeaturesAreMatched(override.RequiredFeatures, dependencies)
		if override.Conditions == nil && override.Segments == nil {
			matchedSelector = requiredMatched
		} else {
			matchedSelector = matchedSelector && requiredMatched
		}
	}
	return matchedSelector
}

func variableValueFromPointer(value *VariableValue) VariableValue {
	if value == nil {
		return nil
	}
	return *value
}

func evaluateGlobalVariable(options EvaluateOptions) Evaluation {
	key := GlobalVariableKey("")
	if options.VariableKey != nil {
		key = GlobalVariableKey(*options.VariableKey)
	}
	base := Evaluation{Type: EvaluationTypeVariable, VariableKey: options.VariableKey, Reason: EvaluationReasonVariableNotFound}
	if options.stickyVariables != nil {
		if value, ok := (*options.stickyVariables)[key]; ok {
			base.Reason = EvaluationReasonSticky
			base.VariableValue = value
			base.variableValueSet = true
			return base
		}
	}
	variable := options.instanceEvaluationDataProvider.GetGlobalVariable(key)
	if variable == nil {
		return base
	}
	base.GlobalVariable = variable
	if variable.Deprecated != nil && *variable.Deprecated {
		options.diagnosticReporter.Warn("variable is deprecated", logDetails{"variableKey": key})
	}
	if !requiredFeaturesAreMatched(variable.RequiredFeatures, options.evaluateDependencies) {
		base.Reason = EvaluationReasonRequiredFeaturesUnmet
		if variable.UseDefaultWhenDisabled {
			base.VariableValue = variable.DefaultValue
			base.variableValueSet = variable.defaultValueSet
		} else if variable.disabledValueSet {
			base.VariableValue = variable.DisabledValue
			base.variableValueSet = true
		}
		return base
	}
	for index, override := range variable.Overrides {
		if variableOverrideIsMatched(override, options.evaluateDependencies) {
			base.Reason = EvaluationReasonVariableOverrideRule
			base.VariableValue = override.Value
			base.variableValueSet = true
			base.VariableOverrideIndex = &index
			base.VariableOverrideKey = override.Key
			base.VariableOverridePath = override.KeyPath
			return base
		}
	}
	base.Reason = EvaluationReasonVariableDefault
	base.VariableValue = variable.DefaultValue
	base.variableValueSet = variable.defaultValueSet
	return base
}

// evaluate evaluates a feature
func evaluate(options EvaluateOptions) Evaluation {
	var evaluation Evaluation

	defer func() {
		if r := recover(); r != nil {
			// Log the panic and return an error evaluation
			options.diagnosticReporter.Error("panic in evaluate", logDetails{
				"panic": r,
			})

			// Return error evaluation
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				VariableKey: options.VariableKey,
				Reason:      EvaluationReasonError,
				Error:       fmt.Errorf("panic: %v", r),
			}
		}
	}()

	// feature not found
	feature := options.instanceEvaluationDataProvider.GetFeature(options.FeatureKey)
	if feature == nil {
		evaluation = Evaluation{
			Type:       options.Type,
			FeatureKey: options.FeatureKey,
			Reason:     EvaluationReasonFeatureNotFound,
		}

		options.diagnosticReporter.Warn("feature not found", logDetails{"evaluation": evaluation})

		return evaluation
	}

	// feature: deprecated
	if options.Type == EvaluationTypeFlag && feature.Deprecated != nil && *feature.Deprecated {
		options.diagnosticReporter.Warn("feature is deprecated", logDetails{
			"featureKey": options.FeatureKey,
		})
	}

	/**
	 * Sticky
	 */
	if options.sticky != nil {
		if stickyFeature, exists := (*options.sticky)[options.FeatureKey]; exists {
			// flag
			if options.Type == EvaluationTypeFlag && stickyFeature.Enabled {
				evaluation = Evaluation{
					Type:       options.Type,
					FeatureKey: options.FeatureKey,
					Reason:     EvaluationReasonSticky,
					Sticky:     &stickyFeature,
					Enabled:    &[]bool{true}[0],
				}

				options.diagnosticReporter.Debug("using sticky enabled", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}

			// variation
			if options.Type == EvaluationTypeVariation && stickyFeature.Variation != nil {
				variationValue := *stickyFeature.Variation
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonSticky,
					Sticky:         &stickyFeature,
					VariationValue: &variationValue,
				}

				options.diagnosticReporter.Debug("using sticky variation", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}

			// variable
			if options.Type == EvaluationTypeVariable && options.VariableKey != nil && stickyFeature.Variables != nil {
				if variableValue, exists := stickyFeature.Variables[*options.VariableKey]; exists {
					evaluation = Evaluation{
						Type:          options.Type,
						FeatureKey:    options.FeatureKey,
						Reason:        EvaluationReasonSticky,
						Sticky:        &stickyFeature,
						VariableKey:   options.VariableKey,
						VariableValue: variableValue,
					}
					evaluation.variableValueSet = true

					options.diagnosticReporter.Debug("using sticky variable", logDetails{
						"evaluation": evaluation,
					})

					return evaluation
				}
			}
		}
	}

	// variableSchema
	var variableSchema *VariableSchema

	if options.VariableKey != nil {
		if feature.VariablesSchema != nil {
			if schema, exists := feature.VariablesSchema[*options.VariableKey]; exists {
				variableSchema = &schema
			}
		}

		// variable schema not found
		if variableSchema == nil {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonVariableNotFound,
				VariableKey: options.VariableKey,
			}

			options.diagnosticReporter.Warn("variable schema not found", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		if variableSchema.Deprecated != nil && *variableSchema.Deprecated {
			options.diagnosticReporter.Warn("variable is deprecated", logDetails{
				"featureKey":  options.FeatureKey,
				"variableKey": *options.VariableKey,
			})
		}
	}

	// variation: no variations
	if options.Type == EvaluationTypeVariation && (feature.Variations == nil || len(feature.Variations) == 0) {
		evaluation = Evaluation{
			Type:       options.Type,
			FeatureKey: options.FeatureKey,
			Reason:     EvaluationReasonNoVariations,
		}

		options.diagnosticReporter.Warn("no variations", logDetails{
			"evaluation": evaluation,
		})

		return evaluation
	}

	/**
	 * Root flag evaluation
	 */
	var flag Evaluation
	if options.Type != EvaluationTypeFlag {
		// needed by variation and variable evaluations
		flag = evaluate(EvaluateOptions{
			evaluateParams: evaluateParams{
				Type:       EvaluationTypeFlag,
				FeatureKey: options.FeatureKey,
			},
			evaluateDependencies: options.evaluateDependencies,
		})

		if flag.Enabled != nil && !*flag.Enabled {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonDisabled,
			}

			// serve variable default value if feature is disabled (if explicitly specified)
			if options.Type == EvaluationTypeVariable {
				if feature != nil && options.VariableKey != nil && feature.VariablesSchema != nil {
					if variableSchema, exists := feature.VariablesSchema[*options.VariableKey]; exists {
						if variableSchema.disabledValueSet {
							// disabledValue: <value>
							evaluation = Evaluation{
								Type:           options.Type,
								FeatureKey:     options.FeatureKey,
								Reason:         EvaluationReasonVariableDisabled,
								VariableKey:    options.VariableKey,
								VariableValue:  variableValueFromPointer(variableSchema.DisabledValue),
								VariableSchema: &variableSchema,
								Enabled:        &[]bool{false}[0],
							}
							evaluation.variableValueSet = true
						} else if variableSchema.UseDefaultWhenDisabled != nil && *variableSchema.UseDefaultWhenDisabled {
							// useDefaultWhenDisabled: true
							evaluation = Evaluation{
								Type:           options.Type,
								FeatureKey:     options.FeatureKey,
								Reason:         EvaluationReasonVariableDefault,
								VariableKey:    options.VariableKey,
								VariableValue:  variableSchema.DefaultValue,
								VariableSchema: &variableSchema,
								Enabled:        &[]bool{false}[0],
							}
							evaluation.variableValueSet = variableSchema.defaultValueSet
						}
					}
				}
			}

			// serve disabled variation value if feature is disabled (if explicitly specified)
			if options.Type == EvaluationTypeVariation && feature != nil && feature.DisabledVariationValue != nil {
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonVariationDisabled,
					VariationValue: feature.DisabledVariationValue,
					Enabled:        &[]bool{false}[0],
				}
			}

			options.diagnosticReporter.Debug("feature is disabled", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}
	}

	/**
	 * Forced
	 */
	forceResult := options.instanceEvaluationDataProvider.GetMatchedForce(feature, options.Context)

	if forceResult.Force != nil {
		force := forceResult.Force
		forceIndex := forceResult.ForceIndex

		// flag
		if options.Type == EvaluationTypeFlag && force.Enabled != nil {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonForced,
				ForceIndex: forceIndex,
				Force:      force,
				Enabled:    force.Enabled,
			}

			options.diagnosticReporter.Debug("forced enabled found", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		// variation
		if options.Type == EvaluationTypeVariation && force.Variation != nil && feature.Variations != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *force.Variation {
					evaluation = Evaluation{
						Type:           options.Type,
						FeatureKey:     options.FeatureKey,
						Reason:         EvaluationReasonForced,
						ForceIndex:     forceIndex,
						Force:          force,
						Variation:      &variation,
						VariationValue: &variation.Value,
					}

					options.diagnosticReporter.Debug("forced variation found", logDetails{
						"evaluation": evaluation,
					})

					return evaluation
				}
			}
		}

		// variable
		if options.VariableKey != nil && force.Variables != nil {
			if variableValue, exists := force.Variables[string(*options.VariableKey)]; exists {
				evaluation = Evaluation{
					Type:           options.Type,
					FeatureKey:     options.FeatureKey,
					Reason:         EvaluationReasonForced,
					ForceIndex:     forceIndex,
					Force:          force,
					VariableKey:    options.VariableKey,
					VariableSchema: variableSchema,
					VariableValue:  variableValue,
				}
				evaluation.variableValueSet = true

				options.diagnosticReporter.Debug("forced variable", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}
		}
	}

	/**
	 * Required
	 */
	if options.Type == EvaluationTypeFlag {
		requiredFeatures := feature.RequiredFeatures
		if len(requiredFeatures) == 0 {
			requiredFeatures = feature.Required
		}
		if len(requiredFeatures) > 0 && !requiredFeaturesAreMatched(requiredFeatures, options.evaluateDependencies) {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonRequired,
				Enabled:    &[]bool{false}[0],
			}
			if len(feature.RequiredFeatures) > 0 {
				evaluation.RequiredFeatures = feature.RequiredFeatures
			} else {
				evaluation.Required = feature.Required
			}

			options.diagnosticReporter.Debug("required features not enabled", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}
	}

	/**
	 * Bucketing
	 */
	// bucketKey
	bucketKey := getBucketKey(getBucketKeyOptions{
		FeatureKey:         options.FeatureKey,
		BucketBy:           feature.BucketBy,
		Context:            options.Context,
		diagnosticReporter: options.diagnosticReporter,
	})

	for _, module := range options.modulesManager.GetAll() {
		if module.BucketKey != nil {
			bucketKey = module.BucketKey(ConfigureBucketKeyOptions{
				FeatureKey: options.FeatureKey,
				Context:    options.Context,
				BucketBy:   feature.BucketBy,
				BucketKey:  bucketKey,
			})
		}
	}

	// bucketValue
	bucketValue := getBucketedNumber(bucketKey)

	for _, module := range options.modulesManager.GetAll() {
		if module.BucketValue != nil {
			bucketValue = module.BucketValue(ConfigureBucketValueOptions{
				FeatureKey:  options.FeatureKey,
				BucketKey:   bucketKey,
				Context:     options.Context,
				BucketValue: bucketValue,
			})
		}
	}

	var matchedTraffic *Traffic
	var matchedAllocation *Allocation

	if options.Type != EvaluationTypeFlag {
		matchedTraffic = options.instanceEvaluationDataProvider.GetMatchedTraffic(feature.Traffic, options.Context)

		if matchedTraffic != nil {
			matchedAllocation = options.instanceEvaluationDataProvider.GetMatchedAllocation(matchedTraffic, bucketValue)
		}
	} else {
		matchedTraffic = options.instanceEvaluationDataProvider.GetMatchedTraffic(feature.Traffic, options.Context)
	}

	if matchedTraffic != nil {
		// percentage: 0
		if matchedTraffic.Percentage == 0 {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonRule,
				BucketKey:   &bucketKey,
				BucketValue: &bucketValue,
				RuleKey:     &matchedTraffic.Key,
				Traffic:     matchedTraffic,
				Enabled:     &[]bool{false}[0],
			}

			options.diagnosticReporter.Debug("matched rule with 0 percentage", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		// flag
		if options.Type == EvaluationTypeFlag {
			// flag: check if mutually exclusive
			if feature.Ranges != nil && len(feature.Ranges) > 0 {
				var matchedRange *Range
				for _, rangeItem := range feature.Ranges {
					if bucketValue >= rangeItem[0] && bucketValue < rangeItem[1] {
						matchedRange = &rangeItem
						break
					}
				}

				// matched
				if matchedRange != nil {
					enabled := true
					if matchedTraffic.Enabled != nil {
						enabled = *matchedTraffic.Enabled
					}

					evaluation = Evaluation{
						Type:        options.Type,
						FeatureKey:  options.FeatureKey,
						Reason:      EvaluationReasonAllocated,
						BucketKey:   &bucketKey,
						BucketValue: &bucketValue,
						RuleKey:     &matchedTraffic.Key,
						Traffic:     matchedTraffic,
						Enabled:     &enabled,
					}

					options.diagnosticReporter.Debug("matched", logDetails{
						"evaluation": evaluation,
					})

					return evaluation
				}

				// no match
				evaluation = Evaluation{
					Type:        options.Type,
					FeatureKey:  options.FeatureKey,
					Reason:      EvaluationReasonOutOfRange,
					BucketKey:   &bucketKey,
					BucketValue: &bucketValue,
					Enabled:     &[]bool{false}[0],
				}

				options.diagnosticReporter.Debug("not matched", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}

			// flag: override from rule
			if matchedTraffic.Enabled != nil {
				evaluation = Evaluation{
					Type:        options.Type,
					FeatureKey:  options.FeatureKey,
					Reason:      EvaluationReasonRule,
					BucketKey:   &bucketKey,
					BucketValue: &bucketValue,
					RuleKey:     &matchedTraffic.Key,
					Traffic:     matchedTraffic,
					Enabled:     matchedTraffic.Enabled,
				}

				options.diagnosticReporter.Debug("override from rule", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}

			// treated as enabled because of matched traffic
			if bucketValue <= matchedTraffic.Percentage {
				evaluation = Evaluation{
					Type:        options.Type,
					FeatureKey:  options.FeatureKey,
					Reason:      EvaluationReasonRule,
					BucketKey:   &bucketKey,
					BucketValue: &bucketValue,
					RuleKey:     &matchedTraffic.Key,
					Traffic:     matchedTraffic,
					Enabled:     &[]bool{true}[0],
				}

				options.diagnosticReporter.Debug("matched traffic", logDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}
		}

		// variation
		if options.Type == EvaluationTypeVariation && feature.Variations != nil {
			// override from rule
			if matchedTraffic.Variation != nil {
				for _, variation := range feature.Variations {
					if variation.Value == *matchedTraffic.Variation {
						evaluation = Evaluation{
							Type:           options.Type,
							FeatureKey:     options.FeatureKey,
							Reason:         EvaluationReasonRule,
							BucketKey:      &bucketKey,
							BucketValue:    &bucketValue,
							RuleKey:        &matchedTraffic.Key,
							Traffic:        matchedTraffic,
							Variation:      &variation,
							VariationValue: &variation.Value,
						}

						options.diagnosticReporter.Debug("override from rule", logDetails{
							"evaluation": evaluation,
						})

						return evaluation
					}
				}
			}

			// Handle variationWeights
			if matchedTraffic.VariationWeights != nil && len(matchedTraffic.VariationWeights) > 0 {
				// Create custom allocation based on variationWeights
				totalWeight := 0
				for _, weight := range matchedTraffic.VariationWeights {
					totalWeight += int(weight)
				}

				if totalWeight > 0 {
					// Calculate which variation the bucket value falls into
					currentWeight := 0

					// Sort variation weights to ensure consistent processing order
					// This matches TypeScript's deterministic object property order
					type variationWeight struct {
						value  VariationValue
						weight int
					}
					var sortedWeights []variationWeight
					for variationValue, weight := range matchedTraffic.VariationWeights {
						sortedWeights = append(sortedWeights, variationWeight{
							value:  variationValue,
							weight: int(weight),
						})
					}

					// Sort by variation value to ensure consistent order
					sort.Slice(sortedWeights, func(i, j int) bool {
						return string(sortedWeights[i].value) < string(sortedWeights[j].value)
					})

					for _, vw := range sortedWeights {
						weightInt := vw.weight
						// Convert percentage to bucket range (0-100000)
						startRange := currentWeight * 100000 / totalWeight
						endRange := (currentWeight + weightInt) * 100000 / totalWeight

						options.diagnosticReporter.Debug("checking variation weight range", logDetails{
							"variationValue": vw.value,
							"weight":         weightInt,
							"startRange":     startRange,
							"endRange":       endRange,
							"bucketValue":    bucketValue,
							"totalWeight":    totalWeight,
						})

						if bucketValue >= startRange && bucketValue < endRange {
							// Find the variation object
							for _, variation := range feature.Variations {
								if variation.Value == vw.value {
									evaluation = Evaluation{
										Type:           options.Type,
										FeatureKey:     options.FeatureKey,
										Reason:         EvaluationReasonAllocated,
										BucketKey:      &bucketKey,
										BucketValue:    &bucketValue,
										RuleKey:        &matchedTraffic.Key,
										Traffic:        matchedTraffic,
										Variation:      &variation,
										VariationValue: &variation.Value,
									}

									options.diagnosticReporter.Debug("allocated variation with custom weights", logDetails{
										"evaluation": evaluation,
									})

									return evaluation
								}
							}
						}
						currentWeight += weightInt
					}
				}
			}

			// regular allocation
			if matchedAllocation != nil && matchedAllocation.Variation != "" {
				for _, variation := range feature.Variations {
					if variation.Value == matchedAllocation.Variation {
						evaluation = Evaluation{
							Type:           options.Type,
							FeatureKey:     options.FeatureKey,
							Reason:         EvaluationReasonAllocated,
							BucketKey:      &bucketKey,
							BucketValue:    &bucketValue,
							RuleKey:        &matchedTraffic.Key,
							Traffic:        matchedTraffic,
							Variation:      &variation,
							VariationValue: &variation.Value,
						}

						options.diagnosticReporter.Debug("allocated variation", logDetails{
							"evaluation": evaluation,
						})

						return evaluation
					}
				}
			}
		}
	}

	// variable
	if options.Type == EvaluationTypeVariable && options.VariableKey != nil {
		// Check if variableSchema is available
		if variableSchema == nil {
			evaluation = Evaluation{
				Type:        options.Type,
				FeatureKey:  options.FeatureKey,
				Reason:      EvaluationReasonVariableNotFound,
				VariableKey: options.VariableKey,
				BucketKey:   &bucketKey,
				BucketValue: &bucketValue,
			}

			options.diagnosticReporter.Debug("variable schema not found", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		// override from rule
		if matchedTraffic != nil {
			if matchedTraffic.VariableOverrides != nil {
				if overrides, exists := matchedTraffic.VariableOverrides[*options.VariableKey]; exists {
					for index, override := range overrides {
						if variableOverrideIsMatched(override, options.evaluateDependencies) {
							overrideIndex := index
							evaluation = Evaluation{
								Type:                  options.Type,
								FeatureKey:            options.FeatureKey,
								Reason:                EvaluationReasonVariableOverrideRule,
								BucketKey:             &bucketKey,
								BucketValue:           &bucketValue,
								RuleKey:               &matchedTraffic.Key,
								Traffic:               matchedTraffic,
								VariableKey:           options.VariableKey,
								VariableSchema:        variableSchema,
								VariableValue:         override.Value,
								VariableOverrideIndex: &overrideIndex,
								VariableOverrideKey:   override.Key,
								VariableOverridePath:  override.KeyPath,
							}
							evaluation.variableValueSet = true

							options.diagnosticReporter.Debug("variable override from rule", logDetails{
								"evaluation": evaluation,
							})

							return evaluation
						}
					}
				}
			}

			if matchedTraffic.Variables != nil {
				if variableValue, exists := matchedTraffic.Variables[string(*options.VariableKey)]; exists {
					evaluation = Evaluation{
						Type:           options.Type,
						FeatureKey:     options.FeatureKey,
						Reason:         EvaluationReasonRule,
						BucketKey:      &bucketKey,
						BucketValue:    &bucketValue,
						RuleKey:        &matchedTraffic.Key,
						Traffic:        matchedTraffic,
						VariableKey:    options.VariableKey,
						VariableSchema: variableSchema,
						VariableValue:  variableValue,
					}
					evaluation.variableValueSet = true

					options.diagnosticReporter.Debug("override from rule", logDetails{
						"evaluation": evaluation,
					})

					return evaluation
				}
			}
		}

		// check variations
		var variationValue *VariationValue

		if forceResult.Force != nil && forceResult.Force.Variation != nil {
			variationValue = forceResult.Force.Variation
		} else if matchedTraffic != nil && matchedTraffic.Variation != nil {
			variationValue = matchedTraffic.Variation
		} else if matchedAllocation != nil && matchedAllocation.Variation != "" {
			variationValue = &matchedAllocation.Variation
		}

		if variationValue != nil && feature.Variations != nil {
			for _, variation := range feature.Variations {
				if variation.Value == *variationValue {
					if variation.VariableOverrides != nil {
						if overrides, exists := variation.VariableOverrides[*options.VariableKey]; exists {
							for index, override := range overrides {
								if variableOverrideIsMatched(override, options.evaluateDependencies) {
									overrideIndex := index
									evaluation = Evaluation{
										Type:        options.Type,
										FeatureKey:  options.FeatureKey,
										Reason:      EvaluationReasonVariableOverrideVariation,
										BucketKey:   &bucketKey,
										BucketValue: &bucketValue,
										RuleKey: func() *RuleKey {
											if matchedTraffic != nil {
												return &matchedTraffic.Key
											}
											return nil
										}(),
										Traffic:               matchedTraffic,
										VariableKey:           options.VariableKey,
										VariableSchema:        variableSchema,
										VariableValue:         override.Value,
										VariableOverrideIndex: &overrideIndex,
										VariableOverrideKey:   override.Key,
										VariableOverridePath:  override.KeyPath,
									}
									evaluation.variableValueSet = true

									options.diagnosticReporter.Debug("variable override from variation", logDetails{
										"evaluation": evaluation,
									})

									return evaluation
								}
							}
						}
					}

					if variation.Variables != nil {
						if variableValue, exists := variation.Variables[*options.VariableKey]; exists {
							evaluation = Evaluation{
								Type:        options.Type,
								FeatureKey:  options.FeatureKey,
								Reason:      EvaluationReasonAllocated,
								BucketKey:   &bucketKey,
								BucketValue: &bucketValue,
								RuleKey: func() *RuleKey {
									if matchedTraffic != nil {
										return &matchedTraffic.Key
									}
									return nil
								}(),
								Traffic:        matchedTraffic,
								VariableKey:    options.VariableKey,
								VariableSchema: variableSchema,
								VariableValue:  variableValue,
							}
							evaluation.variableValueSet = true

							options.diagnosticReporter.Debug("allocated variable", logDetails{
								"evaluation": evaluation,
							})

							return evaluation
						}
					}
				}
			}
		}

		// Check for default value from variable schema
		if variableSchema.defaultValueSet {
			evaluation = Evaluation{
				Type:           options.Type,
				FeatureKey:     options.FeatureKey,
				Reason:         EvaluationReasonVariableDefault,
				BucketKey:      &bucketKey,
				BucketValue:    &bucketValue,
				VariableKey:    options.VariableKey,
				VariableSchema: variableSchema,
				VariableValue:  variableSchema.DefaultValue,
			}
			evaluation.variableValueSet = variableSchema.defaultValueSet

			options.diagnosticReporter.Debug("using default value", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		// Variable not found
		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonVariableNotFound,
			VariableKey: options.VariableKey,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.Debug("variable not found", logDetails{
			"evaluation": evaluation,
		})

		return evaluation
	}

	/**
	 * Nothing matched
	 */
	if options.Type == EvaluationTypeVariation {
		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonNoMatch,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.Debug("no matched variation", logDetails{
			"evaluation": evaluation,
		})

		return evaluation
	}

	if options.Type == EvaluationTypeVariable {
		if variableSchema != nil {
			evaluation = Evaluation{
				Type:           options.Type,
				FeatureKey:     options.FeatureKey,
				Reason:         EvaluationReasonVariableDefault,
				BucketKey:      &bucketKey,
				BucketValue:    &bucketValue,
				VariableKey:    options.VariableKey,
				VariableSchema: variableSchema,
				VariableValue:  variableSchema.DefaultValue,
			}
			evaluation.variableValueSet = variableSchema.defaultValueSet

			options.diagnosticReporter.Debug("using default value", logDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		evaluation = Evaluation{
			Type:        options.Type,
			FeatureKey:  options.FeatureKey,
			Reason:      EvaluationReasonVariableNotFound,
			VariableKey: options.VariableKey,
			BucketKey:   &bucketKey,
			BucketValue: &bucketValue,
		}

		options.diagnosticReporter.Debug("variable not found", logDetails{
			"evaluation": evaluation,
		})

		return evaluation
	}

	evaluation = Evaluation{
		Type:        options.Type,
		FeatureKey:  options.FeatureKey,
		Reason:      EvaluationReasonNoMatch,
		BucketKey:   &bucketKey,
		BucketValue: &bucketValue,
		Enabled:     &[]bool{false}[0],
	}

	options.diagnosticReporter.Debug("nothing matched", logDetails{
		"evaluation": evaluation,
	})

	return evaluation
}
