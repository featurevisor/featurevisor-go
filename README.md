# Featurevisor Go SDK <!-- omit in toc -->

Use the Featurevisor Go SDK to evaluate flags, variations and variables in Go applications.

See the [Go example application](https://github.com/featurevisor/featurevisor-example-go).

## Table of contents <!-- omit in toc -->

- [Installation](#installation)
- [Initialization](#initialization)
  - [Public API](#public-api)
  - [Evaluation types](#evaluation-types)
- [Context](#context)
  - [Setting initial context](#setting-initial-context)
  - [Setting after initialization](#setting-after-initialization)
  - [Replacing existing context](#replacing-existing-context)
  - [Manually passing context](#manually-passing-context)
- [Check if enabled](#check-if-enabled)
- [Getting variation](#getting-variation)
- [Getting variables](#getting-variables)
  - [Getting feature variables](#getting-feature-variables)
  - [Type specific methods](#type-specific-methods)
  - [Getting global variables](#getting-global-variables)
- [Conditions](#conditions)
- [Composition](#composition)
- [Getting evaluations](#getting-evaluations)
- [Sticky](#sticky)
  - [Initialize with sticky](#initialize-with-sticky)
  - [Set sticky afterwards](#set-sticky-afterwards)
- [Setting datafile](#setting-datafile)
  - [Merging by default](#merging-by-default)
  - [Replacing](#replacing)
  - [Loading datafiles on demand](#loading-datafiles-on-demand)
  - [Updating datafile](#updating-datafile)
  - [Interval-based update](#interval-based-update)
- [Diagnostics](#diagnostics)
  - [Levels](#levels)
  - [Handler](#handler)
- [Events](#events)
  - [`datafile_set`](#datafile_set)
  - [`context_set`](#context_set)
  - [`sticky_features_set`](#sticky_features_set)
  - [`sticky_variables_set`](#sticky_variables_set)
  - [`error`](#error)
- [Evaluation details](#evaluation-details)
- [Modules](#modules)
  - [Defining a module](#defining-a-module)
  - [Registering modules](#registering-modules)
- [Child instance](#child-instance)
- [Close](#close)
- [OpenFeature](#openfeature)
- [CLI usage](#cli-usage)
  - [Test](#test)
  - [Benchmark](#benchmark)
  - [Assess distribution](#assess-distribution)
- [GitHub repositories](#github-repositories)
- [Development of this package](#development-of-this-package)
  - [Setting up](#setting-up)
  - [Running tests](#running-tests)
  - [Releasing](#releasing)
- [License](#license)

<!-- FEATUREVISOR_DOCS_BEGIN -->

## Installation

Upgrading an existing application? Follow the [Go migration guide](https://featurevisor.com/docs/migrations/v4/go/).

In your Go application, install the SDK using Go modules:

```bash
go get github.com/featurevisor/featurevisor-go/v4
```

## Initialization

The SDK can be initialized by passing [datafile](https://featurevisor.com/docs/building-datafiles/) content directly:

```go
package main

import (
    "fmt"
    "io"
    "net/http"

    "github.com/featurevisor/featurevisor-go/v4"
)

func main() {
    datafileURL := "https://cdn.yoursite.com/featurevisor-all.json"

    resp, err := http.Get(datafileURL)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        panic(fmt.Errorf("datafile request failed: %s", resp.Status))
    }

    datafileBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }

    var datafileContent featurevisor.DatafileContent
    if err := datafileContent.FromJSON(string(datafileBytes)); err != nil {
        panic(err)
    }

    f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
        Datafile: datafileContent,
    })
    defer f.Close()

    fmt.Println(f.IsEnabled("checkout", featurevisor.Context{"userId": "123"}))
}
```

### Public API

The main runtime API is `featurevisor.CreateFeaturevisor()`:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Datafile: datafileContent,
})
```

Most applications only need `CreateFeaturevisor`, the `Featurevisor` instance type, and `FeaturevisorOptions`. Public extension and observability types include `FeaturevisorModule`, `FeaturevisorDiagnostic`, and the datafile model types.

Concurrent evaluations are safe after an instance is configured. Do not call state-changing methods such as `SetDatafile`, `SetContext`, `SetStickyFeatures`, `SetStickyVariables`, `AddModule`, `RemoveModule`, or `Close` concurrently with evaluations or with each other. Apply those changes from a serialized update path. Module, event, and diagnostic callbacks must synchronize mutable state that they capture.

### Evaluation types

Featurevisor evaluates four kinds of values:

- [**Flag**](#check-if-enabled) (`bool`): whether the feature is enabled or not
- [**Variation**](#getting-variation) (`string`): the variation of the feature (if any)
- [**Feature variable**](#getting-feature-variables): a configuration value owned by a feature
- [**Global variable**](#getting-global-variables): a configuration value evaluated independently of a feature

These evaluations are run against the provided context.

## Context

Contexts are [attribute](https://featurevisor.com/docs/attributes) values you pass to the SDK for evaluating [features](https://featurevisor.com/docs/features) against.

Conditions compare these attributes using the [operators](https://featurevisor.com/docs/operators/), including `empty` and `notEmpty`.

Date comparisons accept portable timestamp strings and `time.Time`. Offsets must stay within 14 hours and both local and UTC years within 0001 to 9999. Fractions are truncated to milliseconds. The zero `time.Time{}` is a valid year 0001 instant, not a missing date. Omit the context entry or use a nil pointer for an unknown date.

They are plain maps:

```go
context := featurevisor.Context{
    "userId": "123",
    "country": "nl",
    // ...other attributes
}
```

### Setting initial context

Set context when you create the instance:

```go
import (
    "github.com/featurevisor/featurevisor-go/v4"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Context: featurevisor.Context{
        "deviceId": "123",
        "country":  "nl",
    },
})
```

This is useful for values that don't change too frequently and available at the time of application startup.

### Setting after initialization

Add context later:

```go
f.SetContext(featurevisor.Context{
    "userId": "234",
})
```

This will merge the new context with the existing one (if already set).

### Replacing existing context

Replace the context by passing `true` as the second argument:

```go
f.SetContext(featurevisor.Context{
    "deviceId": "123",
    "userId":   "234",
    "country":  "nl",
    "browser":  "chrome",
}, true) // replace existing context
```

### Manually passing context

Pass context to one evaluation without changing the instance context:

```go
context := featurevisor.Context{
    "userId": "123",
    "country": "nl",
}

isEnabled := f.IsEnabled("checkout", context)
variation := f.GetVariation("checkout", context)
variableValue := f.GetVariable("checkout", "bgColor", context)
```

When manually passing context, it will merge with existing context set to the SDK instance before evaluating the specific value.

## Check if enabled

Check whether a feature is enabled:

```go
featureKey := "checkout"

isEnabled := f.IsEnabled(featureKey)

if isEnabled {
    // do something
}
```

Pass additional context to one evaluation:

```go
isEnabled := f.IsEnabled(featureKey, featurevisor.Context{
    // ...additional context
})
```

## Getting variation

Evaluate a feature's [variations](https://featurevisor.com/docs/features/#variations):

```go
featureKey := "checkout"

variation := f.GetVariation(featureKey)

if variation != nil && *variation == "treatment" {
    // do something for treatment variation
} else {
    // handle default/control variation
}
```

Pass additional context to one evaluation:

```go
variation := f.GetVariation(featureKey, featurevisor.Context{
    // ...additional context
})
```

## Getting variables

### Getting feature variables

Evaluate a feature's [variables](https://featurevisor.com/docs/features/#variables):

```go
variableKey := "bgColor"

bgColorValue := f.GetVariable("checkout", variableKey)
```

Pass additional context to one evaluation:

```go
bgColorValue := f.GetVariable("checkout", variableKey, featurevisor.Context{
    // ...additional context
})
```

### Type specific methods

Next to generic `GetVariable()` methods, there are also type specific methods available for convenience:

```go
f.GetVariableBoolean(featureKey, variableKey, context)
f.GetVariableString(featureKey, variableKey, context)
f.GetVariableInteger(featureKey, variableKey, context)
f.GetVariableDouble(featureKey, variableKey, context)
f.GetVariableArray(featureKey, variableKey, context)
f.GetVariableObject(featureKey, variableKey, context)
f.GetVariableJSON(featureKey, variableKey, context)
```

Type specific methods do not coerce values. `GetVariableInteger()` returns `nil` for the string `"1"`, and `GetVariableBoolean()` returns `nil` for the string `"true"`.

For typed arrays/objects, use `Into` methods with pointer outputs:

```go
var items []string
_ = f.GetVariableArrayInto(featureKey, variableKey, context, &items)

var cfg MyConfig
_ = f.GetVariableObjectInto(featureKey, variableKey, context, &cfg)
```

`context` and `OverrideOptions` are optional and can be passed before the output pointer.

### Getting global variables

Global variables are independent configuration values. Go uses separate method names because it does not support method overloading:

```go
supportEmail := f.GetGlobalVariableString("supportEmail", context)
settings := f.GetGlobalVariableObject("checkoutSettings", context)
evaluation := f.EvaluateGlobalVariable("supportEmail", context)
```

The complete family includes `GetGlobalVariable`, the Boolean, string, integer, double, array, object and JSON helpers, `GetGlobalVariableKeys`, and `GetVariableEvaluations`. Overrides and conditions are applied for you. See [Variables](https://featurevisor.com/docs/variables/#global-variables) and [Conditions](https://featurevisor.com/docs/conditions/) for how to define them.

Modules should return the same result for the same input during one evaluation. Pass values that change through context.

To see which overrides changed a value, read `VariableOverridePaths` from `EvaluateGlobalVariable`. The singular override fields describe the final match.

## Conditions

[Conditions](https://featurevisor.com/docs/conditions/) are shared by segments, feature and global variable gates, force entries, traffic rules and variable overrides. Generated v4 datafiles use schema version `"4"`. Arrays imply AND. Groups use `and`, `or` and `not`. NOT negates the AND of its direct children. Empty expressions are false.

Leaves compare context attributes, reference a segment by its literal string key, check a feature's enabled state and optional variation, or compare an evaluated variable. Feature enabled checks default to true. Variable leaves may name a feature owner and use compiled path arrays such as `["items", 0, "enabled"]`. Dots in variable keys remain literal. Attribute and variable comparisons use the same operators.

For example, a compiled root gate can combine all four leaf kinds:

```json
{
  "conditions": [
    {"attribute": "country", "operator": "equals", "value": "nl"},
    "eligibleAudience",
    {"feature": "checkout", "variation": "treatment"},
    {"variable": "policy", "path": ["enabled"], "operator": "equals", "value": true}
  ]
}
```

Continue using the ordinary getters:

```go
enabled := f.IsEnabled("checkout", featurevisor.Context{"country": "nl"})
policy := f.GetGlobalVariable("policy")
```

Feature evaluation checks sticky state, then force, then its root gate and traffic rules. Force bypasses the gate. Global variables check sticky state and their gate before reading sources or applying overrides. A missed root gate reports `conditions_unmet` and `Evaluation.Conditions`. Variation and feature variable evaluations retain their ordinary disabled reasons and fallbacks.

Dependencies share the current evaluation session and a combined depth limit of 256 internal handoffs. Cycles and excessive depth fail the whole evaluation. A later independent call starts a fresh session. Ordinary completed false or null source getter results remain values.

## Composition

[Composed variables](https://featurevisor.com/docs/variables/) read other values through ordinary getters with the effective context, modules and sticky state. The list form creates one fresh outer object with literal property names:

```json
{
  "type": "object",
  "compose": [
    {"key": "enabled", "feature": "checkout"},
    {"key": "assignment", "feature": "checkout", "variation": true},
    {"key": "title", "feature": "checkout", "variable": "title"},
    {"key": "policy", "variable": "policy"}
  ]
}
```

The string form, `"compose": "policy"`, is an alias of that literal global variable key. It returns the source value directly, including false, zero, empty strings, arrays and null. JSON sources are decoded exactly once, even when the decoded string resembles JSON.

```go
screen := f.GetGlobalVariable("screen", featurevisor.Context{"country": "nl"})
details := f.EvaluateGlobalVariable("screen")
```

Compositions have no `defaultValue` or `useDefaultWhenDisabled`. They may have root `conditions` and `disabledValue`. An unmet gate skips every source and override, returning the disabled value or null. Whole variable sticky state bypasses both assembly and overrides. An explicit caller default is never forwarded to a source getter.

All sources are read before runtime overrides. Overrides use the ordinary mutation interpreter and provenance fields. The reason is `variable_composed`, or `variable_override_rule` when an override matches. Mutation paths copy changed containers without modifying sources, sibling properties or sticky values.

Treat returned objects and arrays as read only. Composition properties and aliases retain ordinary getter ownership. Descendants may be shared. Only the map composition's outer object is guaranteed to be fresh. Source getter failures retain ordinary fallbacks, while dependency cycles and depth failures invalidate the whole composition.

## Getting evaluations

Evaluate all features available in the instance:

```go
allEvaluations := f.GetFeatureEvaluations(featurevisor.Context{}, nil, featurevisor.OverrideOptions{})

fmt.Printf("%+v\n", allEvaluations)
// {
//   checkout: {
//     enabled: true,
//     variation: "control",
//     variables: {
//       myVariableKey: "myVariableValue",
//     },
//   },
//
//   anotherFeature: {
//     enabled: true,
//     variation: "treatment",
//   }
// }
```

This is handy especially when you want to pass all evaluations from a backend application to the frontend.

## Sticky

Set sticky values to take precedence over feature evaluation against the [datafile](https://featurevisor.com/docs/building-datafiles/):

Sticky values belong to an SDK or child instance. Evaluation options do not accept sticky overrides. Child sticky maps replace parent sticky state rather than inheriting it.

Treat supplied sticky maps and their nested values as read only after handoff. Use the instance or child sticky setters to change them. Mutating a supplied map in place is not supported.

### Initialize with sticky

```go
import (
    "github.com/featurevisor/featurevisor-go/v4"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    StickyFeatures: &featurevisor.StickyFeatures{
        "checkout": {
            Enabled: true,
            // optional
            Variation: func() *featurevisor.VariationValue {
                v := featurevisor.VariationValue("treatment")
                return &v
            }(),
            Variables: map[string]interface{}{
                "myVariableKey": "myVariableValue",
            },
        },
        "anotherFeatureKey": {
            Enabled: false,
        },
    },
})
```

Once initialized with sticky features, the SDK will look for values there first before evaluating the targeting conditions and going through the bucketing process.

### Set sticky afterwards

Set sticky features after initialization:

```go
f.SetStickyFeatures(featurevisor.StickyFeatures{
    "checkout": {
        Enabled: true,
        Variation: func() *featurevisor.VariationValue {
            v := featurevisor.VariationValue("treatment")
            return &v
        }(),
        Variables: map[string]interface{}{
            "myVariableKey": "myVariableValue",
        },
    },
    "anotherFeatureKey": {
        Enabled: false,
    },
}, true) // replace existing sticky features (false by default)

f.SetStickyVariables(featurevisor.StickyVariables{
    "supportEmail": "sticky@example.com",
}, true)
```

## Setting datafile

You may also initialize the SDK without passing `datafile`, and set it later on:

```go
f.SetDatafile(datafileContent)
```

`SetDatafile` accepts either parsed `featurevisor.DatafileContent` or a raw JSON string.

Schema version `"2"` selectors are normalized eagerly before merging. V4 entities are normalized lazily on first read. Informational schema metadata is retained. Partial updates invalidate only incoming entities, while replacement clears normalized caches. Caller definitions are never changed. Keep supplied datafiles read only after handoff.

It returns `true` when the datafile is accepted, including unchanged content, and `false` when the input is rejected or the instance is closed. Rejected input leaves the last accepted datafile in place and reports the existing diagnostic and error event, without a `datafile_set` event. Calls after closing do nothing. Accepted calls still emit `datafile_set`, even when nothing changed. The return value reports acceptance, not whether any features changed.

```go
accepted := f.SetDatafile(datafileContent)
```

### Merging by default

By default, `SetDatafile(datafile)` merges the incoming datafile with the SDK instance's existing datafile:

- incoming `Features`, `Segments`, and global `Variables` override matching keys
- existing `Features`, `Segments`, and global `Variables` that are missing from the incoming datafile are kept
- `Revision`, `SchemaVersion`, and `FeaturevisorVersion` are taken from the incoming datafile

Call `SetDatafile` with different datafiles to merge their features, segments, and global variables.

### Replacing

Pass `true` as the second argument to replace the stored datafile entirely:

```go
f.SetDatafile(datafileContent, true) // replace existing datafile
```

### Loading datafiles on demand

Merge datafiles for different [targets](https://featurevisor.com/docs/targets/) into the same instance. Here, the two datafile values have already been loaded and validated by your application from the same published release:

```go
f.SetDatafile(productsDatafile)
f.SetDatafile(checkoutDatafile)
```

Use immutable release URLs, check HTTP responses and keep shared dependencies consistent. See [Loading datafiles on demand](https://featurevisor.com/docs/use-cases/on-demand-datafiles/) for the complete loading and refresh flow.

### Updating datafile

Each accepted datafile update emits a [`datafile_set`](#datafile-set) event. Subscribe to react to updates.

The triggers for setting the datafile again can be:

- periodic updates based on an interval (like every 5 minutes), or
- reacting to:
  - a specific event in your application (like a user action), or
  - an event served via websocket or server-sent events (SSE)

### Interval-based update

Replace the complete datafile on each refresh so definitions removed upstream are removed from the SDK too. Use merging only when loading partial datafiles intentionally.

```go
import (
    "time"
    "io"
    "net/http"

    "github.com/featurevisor/featurevisor-go/v4"
)

func updateDatafile(f *featurevisor.Featurevisor, datafileURL string) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        resp, err := http.Get(datafileURL)
        if err != nil {
            continue
        }
        datafileBytes, err := io.ReadAll(resp.Body)
        resp.Body.Close()
        if err != nil || resp.StatusCode != http.StatusOK {
            continue
        }

        var datafileContent featurevisor.DatafileContent
        if err := datafileContent.FromJSON(string(datafileBytes)); err != nil {
            continue
        }

        f.SetDatafile(datafileContent, true) // replace
    }
}

// Start the update goroutine
go updateDatafile(f, datafileURL)
```

## Diagnostics

By default, Featurevisor reports diagnostics to the console for `info` level and above with a `[Featurevisor]` prefix.

### Levels

Available diagnostic levels are `fatal`, `error`, `warn`, `info`, and `debug`.

Set the level during initialization or update it afterwards:

```go
logLevel := featurevisor.LogLevelDebug
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    LogLevel: &logLevel,
})

f.SetLogLevel(featurevisor.LogLevelInfo)
```

### Handler

Use `OnDiagnostic` to send structured diagnostics to your observability system:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    LogLevel: &logLevel,
    OnDiagnostic: func(diagnostic featurevisor.FeaturevisorDiagnostic) {
        fmt.Println(diagnostic.Level, diagnostic.Code, diagnostic.Message)
    },
})
```

Modules can also subscribe to diagnostics or report their own from `Setup` via the provided module API.

Every diagnostic has `Level`, `Code`, `Message`, and an object-shaped `Details` map. Optional `Module`, `ModuleName`, and `OriginalError` fields describe provenance. Evaluation metadata belongs in `Details`.

Malformed stringified selectors report `conditions_parse_error` at error level once per selector per incoming entity load. Details contain exactly `conditions` with the original string, `entityType` and `entityKey`. Schema 2 diagnostics run after accepted datafile storage. V4 diagnostics run after caching the first entity read. Handlers may safely read the accepted revision again. A malformed selector fails locally, even through NOT or segment references, without rejecting the datafile or disabling unrelated entities.

Diagnostic handlers are isolated from SDK behaviour. A panic in a handler does not stop other handlers or evaluations.

## Events

Subscribe to SDK events to respond to context, sticky state and datafile changes.

Subscribe to instance lifecycle events:

### `datafile_set`

```go
unsubscribe := f.On(featurevisor.EventNameDatafileSet, func(details featurevisor.EventDetails) {
    revision := details["revision"]               // new revision
    previousRevision := details["previousRevision"]
    revisionChanged := details["revisionChanged"] // true if revision has changed

    // list of feature keys that have new updates,
    // and you should re-evaluate them
    features := details["features"]
    variables := details["variables"]

    // handle here
})

// stop listening to the event
unsubscribe()
```

The `features` and `variables` arrays include direct changes and transitive dependencies through conditions, segments and composition sources, including additions and removals.

- added, or
- updated, or
- removed

compared to the previous datafile content that existed in the SDK instance.

### `context_set`

```go
unsubscribe := f.On(featurevisor.EventNameContextSet, func(details featurevisor.EventDetails) {
    replaced := details["replaced"] // true if context was replaced
    context := details["context"]   // the new context

    fmt.Println("Context set")
})
```

### `sticky_features_set`

```go
unsubscribe := f.On(featurevisor.EventNameStickyFeaturesSet, func(details featurevisor.EventDetails) {
    replaced := details["replaced"] // true if sticky features got replaced
    features := details["features"] // list of all affected feature keys

    fmt.Println("Sticky features set")
})
```

### `sticky_variables_set`

```go
unsubscribe := f.On(featurevisor.EventNameStickyVariablesSet, func(details featurevisor.EventDetails) {
    variables := details["variables"]
    replaced := details["replaced"]
    fmt.Println(variables, replaced)
})
```

### `error`

```go
unsubscribe := f.On(featurevisor.EventNameError, func(details featurevisor.EventDetails) {
    diagnostic := details["diagnostic"]
    fmt.Println(diagnostic)
})
```

The `error` event is emitted for diagnostics whose level is `error`.

## Evaluation details

Inspect how variations and variables are evaluated against a context:

```go
// flag
evaluation := f.EvaluateFlag(featureKey, context)

// variation
evaluation := f.EvaluateVariation(featureKey, context)

// variable
evaluation := f.EvaluateVariable(featureKey, variableKey, context)
```

The returned object will always contain the following properties:

- `Type`: flag, variation or variable
- `FeatureKey`: the feature key when the evaluation has a feature owner
- `Reason`: the reason how the value was evaluated

And optionally these properties depending on whether you are evaluating a feature variation or a variable:

- `BucketValue`: the bucket value between 0 and 100,000
- `RuleKey`: the rule key
- `Error`: the error object
- `Enabled`: if feature itself is enabled or not
- `Variation`: the variation object
- `VariationValue`: the variation value
- `VariableKey`: the variable key
- `VariableValue`: the variable value
- `VariableSchema`: the variable schema
- `VariableOverrideIndex`: index of matched variable override when applicable
- `Conditions`: normalized root conditions when the gate is unmet
- `GlobalVariable`: the global variable definition when applicable
- `StickyFeature`: the sticky feature result when applicable
- `VariableOverrideKey`: key of the final matched variable override
- `VariableOverridePath`: path of the final matched variable override
- `VariableOverridePaths`: all matched accumulated override paths in execution order

## Modules

The before evaluation and after evaluation phases each use the module membership at the start of that phase. Where an evaluation uses bucketing, the bucket key and bucket value phases each use their own starting membership. Adding or removing a module during a phase does not change the iteration already in progress. The next phase uses the membership then installed. A module added before the after phase can receive after callbacks for evaluations already in progress, even if it did not receive their before callbacks. A module removed before the after phase receives no after callback.

Modules allow you to intercept the evaluation process and customize it further as per your needs.

For every feature or global variable evaluation, all `BeforeEvaluation` callbacks run in registration order before evaluation, followed by all `AfterEvaluation` callbacks in registration order. Condition and composition dependency getters use the same pipeline. An accumulated override program is one evaluation, so callbacks wrap the complete program rather than individual operations.

`EvaluateOptions.DependencyOf` and `DependencyPurpose` are absent for an application call. Internal reads identify the immediate requester through `Type`, optional `FeatureKey` and optional `VariableKey`. Purpose is `condition` for gating reads and `composition` for consumed values. Tracking modules can skip condition reads while retaining composition exposures. Conditions preserve their written order and short circuit, so put cheap comparisons first when appropriate.

Equivalent dependency reads are memoized within one public call, separating purpose, requester, effective context and current SDK state. Condition snapshots are owned. Composition values keep ordinary getter ownership. Callback options do not retain private sessions. Independent public calls from callbacks start fresh sessions.

### Defining a module

A module is a simple struct with a recommended unique `Name` and optional functions:

If `Setup` panics, the module is not registered. Featurevisor removes subscriptions created during setup, reports `module_setup_error`, and calls `Close` when present.

```go
import (
    "github.com/featurevisor/featurevisor-go/v4"
)

myCustomModule := &featurevisor.FeaturevisorModule{
    // recommended for diagnostics and removal
    Name: "my-custom-module",

    // rest of the properties below are all optional per module
    Setup: func(api featurevisor.FeaturevisorModuleApi) {
        api.ReportDiagnostic(featurevisor.FeaturevisorModuleReportedDiagnostic{
            Level:   featurevisor.LogLevelInfo,
            Code:    "module_ready",
            Message: "Module is ready",
        })
    },

    // before a feature or global variable evaluation
    BeforeEvaluation: func(options featurevisor.EvaluateOptions) featurevisor.EvaluateOptions {
        // update context before evaluation
        if options.Context == nil {
            options.Context = featurevisor.Context{}
        }
        options.Context["someAdditionalAttribute"] = "value"
        return options
    },

    // after a feature or global variable evaluation
    AfterEvaluation: func(evaluation featurevisor.Evaluation, options featurevisor.EvaluateOptions) featurevisor.Evaluation {
        if evaluation.Reason == "error" {
            // log error
            return evaluation
        }
        return evaluation
    },

    // configure bucket key
    BucketKey: func(options featurevisor.ConfigureBucketKeyOptions) featurevisor.BucketKey {
        // return custom bucket key
        return options.BucketKey
    },

    // configure bucket value (between 0 and 100,000)
    BucketValue: func(options featurevisor.ConfigureBucketValueOptions) featurevisor.BucketValue {
        // return custom bucket value
        return options.BucketValue
    },

    Close: func() {
        // clean up module resources
    },
}
```

### Registering modules

Register modules when you create the instance:

```go
import (
    "github.com/featurevisor/featurevisor-go/v4"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Modules: []*featurevisor.FeaturevisorModule{
        myCustomModule,
    },
})
```

Or after initialization:

```go
removeModule := f.AddModule(myCustomModule)
removeModule()
```

## Child instance

A child snapshots the parent keys that exist when it is spawned. Child values win for those keys. Parent keys introduced later are still inherited. Calling `Close()` removes both child-owned listeners and subscriptions delegated to the parent.

When dealing with purely client-side applications, it is understandable that there is only one user involved, like in browser or mobile applications.

But when using Featurevisor SDK in server-side applications, where a single server instance can handle multiple user requests simultaneously, it is important to isolate the context for each request.

That's where child instances come in handy:

```go
childF := f.Spawn(featurevisor.Context{
    // user or request specific context
    "userId": "123",
})
```

Pass the child instance to the request handler to evaluate features for that user:

```go
isEnabled := childF.IsEnabled("checkout")
variation := childF.GetVariation("checkout")
variableValue := childF.GetVariable("checkout", "bgColor")
```

Similar to parent SDK, child instances also support several additional methods:

- `SetContext`
- `SetStickyFeatures`
- `EvaluateFlag`
- `IsEnabled`
- `EvaluateVariation`
- `GetVariation`
- `EvaluateVariable`
- `GetVariable`
- `GetVariableBoolean`
- `GetVariableString`
- `GetVariableInteger`
- `GetVariableDouble`
- `GetVariableArray`
- `GetVariableArrayInto`
- `GetVariableObject`
- `GetVariableObjectInto`
- `GetVariableJSON`
- `On`
- `Close`

## Close

Both primary and child instances support a `.Close()` method, that removes forgotten event listeners (via `On` method) and cleans up any potential memory leaks.

```go
f.Close()
```

## OpenFeature

The OpenFeature provider is a separate Go module, so applications that do not use OpenFeature do not receive its dependencies. The provider requires Go 1.26 or newer and uses OpenFeature Go SDK 1.19.0. The core Featurevisor SDK still supports Go 1.21:

```bash
go get github.com/featurevisor/featurevisor-go/openfeature/v4
```

```go
import (
    "context"

    featurevisor "github.com/featurevisor/featurevisor-go/v4"
    featurevisorof "github.com/featurevisor/featurevisor-go/openfeature/v4"
    of "github.com/open-feature/go-sdk/openfeature"
)

provider := featurevisorof.NewProvider(featurevisorof.Options{
    FeaturevisorOptions: featurevisor.FeaturevisorOptions{
        Datafile: datafileContent,
    },
})

if err := of.SetProviderAndWait(provider); err != nil {
    panic(err)
}

client := of.NewClient("")
enabled, err := client.BooleanValue(
    context.Background(),
    "checkout",
    false,
    of.NewEvaluationContext("user-123", map[string]any{"country": "nl"}),
)
```

Use `checkout` for a flag, `checkout:variation` for its variation, `checkout:title` for a feature variable, and `variable:supportEmail` for a global variable. Boolean variables use the boolean resolver. Arrays, objects, and JSON variables use the object resolver.

OpenFeature's targeting key maps to `userId` by default. `TargetingKeyField`, `KeySeparator`, `VariationKey`, and `GlobalVariablePrefix` can customize the mapping. The global prefix defaults to `variable` and cannot contain the separator. The provider's separate module follows the Go version requirement of the official OpenFeature Go SDK. A provider call receiving a `FlattenedContext` with an explicitly empty `targetingKey` overrides the configured field. The upstream Go client omits empty targeting keys when flattening `EvaluationContext`, so `NewEvaluationContext("", attributes)` cannot express the difference between absent and explicitly empty. To override an existing field with an empty key through the client, include `"targetingKey": ""` in its attributes. If a nonempty targeting key is inherited into the final context, clear it at its source first. A narrower empty key may not remove it.

Root `conditions_unmet` outcomes map to OpenFeature `DISABLED`. Composed values and matched overrides map to `TARGETING_MATCH`. The provider does not decode composed JSON values again.

Reuse an existing Featurevisor instance:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Datafile: datafileContent,
})
provider := featurevisorof.NewProvider(featurevisorof.Options{
    Featurevisor: f,
})
```

The caller owns an instance passed this way. Provider shutdown does not close it. Call `fv.Close()` when every consumer is finished with it. When the provider creates the instance from `FeaturevisorOptions`, the provider owns and closes it. If both fields are supplied, `Featurevisor` takes precedence over `FeaturevisorOptions`.

An invalid initial datafile in an owned instance makes `SetProviderAndWait` return a parse error, independently of the diagnostic log level. Valid updates clear the error and emit readiness. Shutdown removes provider subscriptions and closes only an owned instance.

See the [OpenFeature provider guide](https://featurevisor.com/docs/sdks/openfeature/) for resolution reasons, errors, metadata, tracking, lifecycle, and providers for other languages.

## CLI usage

Build the executable from this SDK repository:

```bash
go build -o featurevisor ./cmd
```

The native test runner requires `@featurevisor/cli` 4.0.0 or later and Node.js 24 or later. Install the Node.js CLI in your Featurevisor project:

```bash
npm install --save-dev @featurevisor/cli@^4.0.0
```

The native CLI supports `test`, `benchmark` and `assess-distribution`. Node.js remains responsible for parsing definitions, listing tests through the execution protocol, expanding matrices and generating datafiles through `npx featurevisor`. Evaluations run through this Go SDK. Use the Node.js CLI directly for authoring commands such as `init`, `lint` and `build`. The SDK runtime has no Node.js dependency.

Options use kebab case, such as `--project-directory-path`. The camelCase forms from earlier releases still work, and `--root-directory-path` is an alias for `--project-directory-path`. Pass values as `--name=value` or `--name value`. Repeat `--target` to select several targets in order. Run `<command> --help` to list every option.

Quote JSON and paths containing spaces as a single shell argument. Duplicate targets are evaluated once, at their first position. Extra positional arguments and unknown options are rejected.

### Test

```bash
./featurevisor test \
  --project-directory-path=/path/to/featurevisor-project \
  --only-failures
```

See [Testing](https://featurevisor.com/docs/testing/) for authoring specs and [Checks](https://featurevisor.com/docs/testing/#checks) for check syntax and operators.

Segment owners and checks use ordinary SDK gates, including feature and variable dependencies, fixed `at`, context and sticky state. Selected Target segment specializations remain authoritative. Compiled execution definitions restore missing segments. Attribute only segment chains can run without an environment. Dependent segments need their selected datafile. Every transitive check reference is preflighted before getters run. Synthetic segment gates are excluded from coverage. Testing does not automatically run lint.

Useful test options:

- `--key-pattern=<pattern>` and `--assertion-pattern=<pattern>` filter tests.
- `--entity-type=scenario` selects scenarios. `--set=<set>` selects one project set. Omitting `--set` runs all sets.
- `--case=<id>` reruns one expanded case. Copy the opaque ID or rerun command from a failure report.
- `--json --pretty` produces an indented JSON report.
- `--coverage` reports observed rule and override selections. `--uncovered` lists features and global variables without associated specs.
- `--only-failures`, `--quiet`, `--verbose` and `--show-datafile` control diagnostic output.

### Benchmark

The examples below use a project without environments. Add `--environment=<name>` if your project configures environments. In a sets project, omit `--set` to run every set or pass `--set=<name>`. Repeat `--target=<key>` to evaluate specific target datafiles. The Node.js CLI validates these options and builds the datafiles. Go evaluates them. Replace `checkout` with a feature defined in your project.

```bash
./featurevisor benchmark \
  --project-directory-path=/path/to/featurevisor-project \
  --feature=checkout \
  --context='{"country":"nl"}' \
  --n=1000
```

Use `--variation` for a variation or `--variable=<key>` for a feature variable. Omit `--feature` to benchmark a global variable. Do not combine `--variation` with `--variable`. See [Benchmarking](https://featurevisor.com/docs/cli/#benchmarking).

### Assess distribution

```bash
./featurevisor assess-distribution \
  --project-directory-path=/path/to/featurevisor-project \
  --feature=checkout \
  --populate-uuid=userId \
  --n=1000
```

Repeat `--populate-uuid=<key>` or `--populate-uuid <key>` to generate UUID values for multiple context attributes. `--n` and `-n` both accept a positive integer and default to 1000 evaluations in this native CLI. See [Assess distribution](https://featurevisor.com/docs/cli/#assess-distribution).

## GitHub repositories

- SDK: [featurevisor/featurevisor-go](https://github.com/featurevisor/featurevisor-go)
- Example application: [featurevisor/featurevisor-example-go](https://github.com/featurevisor/featurevisor-example-go)

<!-- FEATUREVISOR_DOCS_END -->

<a id="getting-all-evaluations"></a>

The aggregate evaluation reference has moved to [Getting evaluations](#getting-evaluations).

<a id="sticky-features-and-variables"></a>

The sticky state reference has moved to [Sticky](#sticky).

## Development of this package

### Setting up

Clone the repository, and install the dependencies using Go modules:

```bash
go mod download
```

### Running tests

```bash
make check
make test-example-1
```

The native tests cover runtime conformance and project integration.

Run `go test -run TestReadmeInitialization .` to compile and execute the actual initialization snippet against a local HTTP fixture. This check also runs in the ordinary SDK suite and CI.

### Releasing

- Tag the core SDK as `v4.x.y`.
- Tag the provider module separately as `openfeature/v4.x.y`.
- Release the core SDK before the provider that depends on it.
- Run `make verify-packages` before creating either release.
- Create the matching releases on [GitHub](https://github.com/featurevisor/featurevisor-go/releases).

## License

MIT © [Fahad Heylaal](https://fahad19.com)
