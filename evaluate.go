package featurevisor

import (
	"fmt"
	"sort"
)

// EvaluateParams contains parameters for evaluation
type EvaluateParams struct {
	Type        EvaluationType
	FeatureKey  FeatureKey
	VariableKey *VariableKey
}

// EvaluateDependencies contains dependencies for evaluation
type EvaluateDependencies struct {
	Context        Context
	Logger         *Logger
	HooksManager   *HooksManager
	DatafileReader *DatafileReader

	// OverrideOptions
	Sticky *StickyFeatures

	DefaultVariationValue *VariationValue
	DefaultVariableValue  VariableValue
}

// EvaluateOptions contains all options for evaluation
type EvaluateOptions struct {
	EvaluateParams
	EvaluateDependencies
}

// EvaluateWithHooks evaluates a feature with hooks
func EvaluateWithHooks(opts EvaluateOptions) Evaluation {
	var evaluation Evaluation

	defer func() {
		if r := recover(); r != nil {
			opts.Logger.Error("panic during evaluation", LogDetails{
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

	hooksManager := opts.HooksManager
	hooks := hooksManager.GetAll()

	// run before hooks
	options := opts
	for _, hook := range hooks {
		if hook.Before != nil {
			options = hook.Before(options)
		}
	}

	// evaluate
	evaluation = Evaluate(options)

	// default: variation
	if opts.DefaultVariationValue != nil &&
		evaluation.Type == EvaluationTypeVariation &&
		evaluation.VariationValue == nil {
		evaluation.VariationValue = opts.DefaultVariationValue
	}

	// default: variable
	if opts.DefaultVariableValue != nil &&
		evaluation.Type == EvaluationTypeVariable &&
		evaluation.VariableValue == nil {
		evaluation.VariableValue = opts.DefaultVariableValue
	}

	// run after hooks
	for _, hook := range hooks {
		if hook.After != nil {
			evaluation = hook.After(evaluation, options)
		}
	}

	return evaluation
}

// Evaluate evaluates a feature
func Evaluate(options EvaluateOptions) Evaluation {
	var evaluation Evaluation

	defer func() {
		if r := recover(); r != nil {
			// Log the panic and return an error evaluation
			options.Logger.Error("panic in evaluate", LogDetails{
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
	feature := options.DatafileReader.GetFeature(options.FeatureKey)
	if feature == nil {
		evaluation = Evaluation{
			Type:       options.Type,
			FeatureKey: options.FeatureKey,
			Reason:     EvaluationReasonFeatureNotFound,
		}

		options.Logger.Warn("feature not found", LogDetails{
			"featureKey": options.FeatureKey,
		})

		return evaluation
	}

	// feature: deprecated
	if options.Type == EvaluationTypeFlag && feature.Deprecated != nil && *feature.Deprecated {
		options.Logger.Warn("feature is deprecated", LogDetails{
			"featureKey": options.FeatureKey,
		})
	}

	/**
	 * Sticky
	 */
	if options.Sticky != nil {
		if stickyFeature, exists := (*options.Sticky)[options.FeatureKey]; exists {
			// flag
			if options.Type == EvaluationTypeFlag && stickyFeature.Enabled {
				evaluation = Evaluation{
					Type:       options.Type,
					FeatureKey: options.FeatureKey,
					Reason:     EvaluationReasonSticky,
					Sticky:     &stickyFeature,
					Enabled:    &[]bool{true}[0],
				}

				options.Logger.Debug("using sticky enabled", LogDetails{
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

				options.Logger.Debug("using sticky variation", LogDetails{
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

					options.Logger.Debug("using sticky variable", LogDetails{
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

			options.Logger.Warn("variable schema not found", LogDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		if variableSchema.Deprecated != nil && *variableSchema.Deprecated {
			options.Logger.Warn("variable is deprecated", LogDetails{
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

		options.Logger.Warn("no variations", LogDetails{
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
		flag = Evaluate(EvaluateOptions{
			EvaluateParams: EvaluateParams{
				Type:       EvaluationTypeFlag,
				FeatureKey: options.FeatureKey,
			},
			EvaluateDependencies: options.EvaluateDependencies,
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
						if variableSchema.DisabledValue != nil {
							// disabledValue: <value>
							evaluation = Evaluation{
								Type:           options.Type,
								FeatureKey:     options.FeatureKey,
								Reason:         EvaluationReasonVariableDisabled,
								VariableKey:    options.VariableKey,
								VariableValue:  *variableSchema.DisabledValue,
								VariableSchema: &variableSchema,
								Enabled:        &[]bool{false}[0],
							}
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

			options.Logger.Debug("feature is disabled", LogDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}
	}

	/**
	 * Forced
	 */
	forceResult := options.DatafileReader.GetMatchedForce(feature, options.Context)

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

			options.Logger.Debug("forced enabled found", LogDetails{
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

					options.Logger.Debug("forced variation found", LogDetails{
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

				options.Logger.Debug("forced variable", LogDetails{
					"evaluation": evaluation,
				})

				return evaluation
			}
		}
	}

	/**
	 * Required
	 */
	if options.Type == EvaluationTypeFlag && feature.Required != nil && len(feature.Required) > 0 {
		requiredFeaturesAreEnabled := true

		for _, required := range feature.Required {
			var requiredKey FeatureKey
			var requiredVariation *VariationValue

			if requiredStr, ok := required.(string); ok {
				requiredKey = FeatureKey(requiredStr)
			} else if requiredWithVar, ok := required.(RequiredWithVariation); ok {
				requiredKey = requiredWithVar.Key
				requiredVariation = &requiredWithVar.Variation
			}

			requiredEvaluation := Evaluate(EvaluateOptions{
				EvaluateParams: EvaluateParams{
					Type:       EvaluationTypeFlag,
					FeatureKey: requiredKey,
				},
				EvaluateDependencies: options.EvaluateDependencies,
			})
			requiredIsEnabled := requiredEvaluation.Enabled != nil && *requiredEvaluation.Enabled

			if !requiredIsEnabled {
				requiredFeaturesAreEnabled = false
				break
			}

			if requiredVariation != nil {
				requiredVariationEvaluation := Evaluate(EvaluateOptions{
					EvaluateParams: EvaluateParams{
						Type:       EvaluationTypeVariation,
						FeatureKey: requiredKey,
					},
					EvaluateDependencies: options.EvaluateDependencies,
				})

				var requiredVariationValue *VariationValue

				if requiredVariationEvaluation.VariationValue != nil {
					requiredVariationValue = requiredVariationEvaluation.VariationValue
				} else if requiredVariationEvaluation.Variation != nil {
					requiredVariationValue = &requiredVariationEvaluation.Variation.Value
				}

				if requiredVariationValue == nil || *requiredVariationValue != *requiredVariation {
					requiredFeaturesAreEnabled = false
					break
				}
			}
		}

		if !requiredFeaturesAreEnabled {
			evaluation = Evaluation{
				Type:       options.Type,
				FeatureKey: options.FeatureKey,
				Reason:     EvaluationReasonRequired,
				Required:   feature.Required,
				Enabled:    &[]bool{requiredFeaturesAreEnabled}[0],
			}

			options.Logger.Debug("required features not enabled", LogDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}
	}

	/**
	 * Bucketing
	 */
	// bucketKey
	bucketKey := GetBucketKey(GetBucketKeyOptions{
		FeatureKey: options.FeatureKey,
		BucketBy:   feature.BucketBy,
		Context:    options.Context,
		Logger:     options.Logger,
	})

	for _, hook := range options.HooksManager.GetAll() {
		if hook.BucketKey != nil {
			bucketKey = hook.BucketKey(ConfigureBucketKeyOptions{
				FeatureKey: options.FeatureKey,
				Context:    options.Context,
				BucketBy:   feature.BucketBy,
				BucketKey:  bucketKey,
			})
		}
	}

	// bucketValue
	bucketValue := GetBucketedNumber(bucketKey)

	for _, hook := range options.HooksManager.GetAll() {
		if hook.BucketValue != nil {
			bucketValue = hook.BucketValue(ConfigureBucketValueOptions{
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
		matchedTraffic = options.DatafileReader.GetMatchedTraffic(feature.Traffic, options.Context)

		if matchedTraffic != nil {
			matchedAllocation = options.DatafileReader.GetMatchedAllocation(matchedTraffic, bucketValue)
		}
	} else {
		matchedTraffic = options.DatafileReader.GetMatchedTraffic(feature.Traffic, options.Context)
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

			options.Logger.Debug("matched rule with 0 percentage", LogDetails{
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

					options.Logger.Debug("matched", LogDetails{
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

				options.Logger.Debug("not matched", LogDetails{
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

				options.Logger.Debug("override from rule", LogDetails{
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

				options.Logger.Debug("matched traffic", LogDetails{
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

						options.Logger.Debug("override from rule", LogDetails{
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

						options.Logger.Debug("checking variation weight range", LogDetails{
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

									options.Logger.Debug("allocated variation with custom weights", LogDetails{
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

						options.Logger.Debug("allocated variation", LogDetails{
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

			options.Logger.Debug("variable schema not found", LogDetails{
				"evaluation": evaluation,
			})

			return evaluation
		}

		// override from rule
		if matchedTraffic != nil && matchedTraffic.Variables != nil {
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

				options.Logger.Debug("override from rule", LogDetails{
					"evaluation": evaluation,
				})

				return evaluation
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
							for _, override := range overrides {
								matched := false

								if override.Conditions != nil {
									matched = options.DatafileReader.AllConditionsAreMatched(override.Conditions, options.Context)
								} else if override.Segments != nil {
									// Parse segments if they come from JSON unmarshaling
									parsedSegments := options.DatafileReader.parseSegmentsIfStringified(override.Segments)
									matched = options.DatafileReader.AllSegmentsAreMatched(parsedSegments, options.Context)
								}

								if matched {
									evaluation = Evaluation{
										Type:        options.Type,
										FeatureKey:  options.FeatureKey,
										Reason:      EvaluationReasonVariableOverride,
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
										VariableValue:  override.Value,
									}

									options.Logger.Debug("variable override", LogDetails{
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

							options.Logger.Debug("allocated variable", LogDetails{
								"evaluation": evaluation,
							})

							return evaluation
						}
					}
				}
			}
		}

		// Check for default value from variable schema
		if variableSchema.DefaultValue != nil {
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

			options.Logger.Debug("using default value", LogDetails{
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

		options.Logger.Debug("variable not found", LogDetails{
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

		options.Logger.Debug("no matched variation", LogDetails{
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

			options.Logger.Debug("using default value", LogDetails{
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

		options.Logger.Debug("variable not found", LogDetails{
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

	options.Logger.Debug("nothing matched", LogDetails{
		"evaluation": evaluation,
	})

	return evaluation
}
